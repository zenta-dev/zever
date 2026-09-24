package sqlite

import (
	"container/heap"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/vectorstore"
)

var (
	embeddingCodec = codec.JSONCodec[[]float32]{}
	metadataCodec  = codec.JSONCodec[map[string]any]{}
)

// maxScanRows caps the number of rows sqlite Query will scan brute-force.
// sqlite has no vector index, so Query is O(n). Without a cap a large table
// causes long latency and high memory pressure. Callers needing larger
// corpora should use pgvector/qdrant.
const maxScanRows = 10000

var memoryCounter uint64

// DefaultDDLTimeout bounds DDL during construction.
const DefaultDDLTimeout = 5 * time.Second

// Store implements vectorstore.VectorStore backed by SQLite.
type Store struct {
	db  *sql.DB
	mu  sync.RWMutex
	dim int
}

// New creates a SQLite-backed VectorStore from Options.
// An empty DSN defaults to vectorstore.DefaultSQLiteDSN.
func New(o vectorstore.Options) (vectorstore.VectorStore, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	dsn := o.DSN
	if dsn == "" {
		dsn = vectorstore.DefaultSQLiteDSN
	}

	if dsn == ":memory:" {
		n := atomic.AddUint64(&memoryCounter, 1)
		dsn = fmt.Sprintf("file:zen-vectorstore-%d?mode=memory&cache=shared", n)
	} else {
		if err := validateDSN(dsn); err != nil {
			return nil, err
		}
	}

	// sql.Open only fails for an unknown driver name; "sqlite" is registered
	// by the blank import above, so this branch is unreachable in practice
	// and intentionally left without a dedicated test.
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	// A single pooled connection serializes access at the Go level. This
	// keeps :memory: shared-cache databases (process-local state tied to
	// whichever connection references them) from racing DDL against
	// themselves on a second pooled connection, and avoids SQLITE_BUSY on
	// concurrent writes without a busy-timeout pragma.
	d.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultDDLTimeout)
	defer cancel()

	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		embedding BLOB NOT NULL,
		metadata BLOB
	)`); err != nil {
		_ = d.Close() //nolint:gosec // G104: cleanup on error path
		return nil, fmt.Errorf("sqlite: create table: %w", err)
	}

	return &Store{db: d}, nil
}

// execer abstracts *sql.DB and *sql.Tx so upsert logic can run against either
// a bare connection or an in-flight transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Upsert inserts or replaces a vector in the store.
func (s *Store) Upsert(ctx context.Context, vec vectorstore.Vector) error {
	if err := vec.Validate(); err != nil {
		return fmt.Errorf("sqlite: upsert: %w", err)
	}

	if err := s.upsertOne(ctx, s.db, vec); err != nil {
		return err
	}

	return nil
}

// UpsertBatch inserts or replaces all of vecs in a single transaction.
func (s *Store) UpsertBatch(ctx context.Context, vecs []vectorstore.Vector) error {
	for i, vec := range vecs {
		if err := vec.Validate(); err != nil {
			return fmt.Errorf("sqlite: upsert batch: index %d: %w", i, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: upsert batch: begin: %w", err)
	}

	for _, vec := range vecs {
		if err := s.upsertOne(ctx, tx, vec); err != nil {
			_ = tx.Rollback() //nolint:gosec // G104: best-effort rollback on error path

			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: upsert batch: commit: %w", err)
	}

	return nil
}

// upsertOne inserts or replaces vec using exec, which may be s.db or an
// in-flight *sql.Tx, so Upsert and UpsertBatch share identical per-item logic.
func (s *Store) upsertOne(ctx context.Context, exec execer, vec vectorstore.Vector) error {
	if len(vec.Embedding) == 0 {
		return fmt.Errorf("sqlite: upsert: %w", vectorstore.ErrEmptyEmbedding)
	}

	if err := s.checkAndSetDim(len(vec.Embedding)); err != nil {
		return err
	}

	blob := encodeEmbedding(vec.Embedding)

	metaBlob, err := encodeMetadata(vec.Metadata)
	if err != nil {
		return fmt.Errorf("sqlite: encode metadata: %w", err)
	}

	if _, err := exec.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		vec.ID, blob, metaBlob,
	); err != nil {
		return fmt.Errorf("sqlite: upsert: %w", err)
	}

	return nil
}

// Delete removes a vector by ID from the store.
func (s *Store) Delete(ctx context.Context, id string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT 1 FROM vectors WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	exists := rows.Next()

	if err := rows.Close(); err != nil { //nolint:sqlclosecheck // closed immediately (not deferred) so its error surfaces before the rows.Err() check below
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	// rows.Err() distinguishes "no more rows" (id absent) from a mid-iteration
	// driver failure, so reaching here with exists==false reliably means the
	// id was absent.
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	if !exists {
		// Pointer chain required: callers match with a *NotFoundError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		notFound := error(&vectorstore.NotFoundError{ID: id})
		return fmt.Errorf("sqlite: delete: %w", notFound)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM vectors WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete: %w", err)
	}

	return nil
}

// Query finds the top-K nearest vectors to the given embedding.
//
// sqlite is a brute-force fallback without a vector index, so this scans every
// row and keeps only topK candidates in a bounded heap (O(topK) memory). For
// large corpora use pgvector/qdrant. To avoid OOM/latency on
// accidental large tables, the scan is capped at maxScanRows (10000) and
// returns an error if exceeded.
func (s *Store) Query(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if err := s.validateQueryDim(embedding); err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = vectorstore.DefaultTopK
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, embedding, metadata FROM vectors`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	h, err := s.scanRows(ctx, rows, embedding, topK)
	if err != nil {
		_ = rows.Close()
		return nil, err
	}

	// No further rows.Err()/Close check here: scanRows ends with its own
	// rows.Err() check, and database/sql auto-closes rows on EOF, funneling
	// any auto-close failure into rows.Err() (so scanRows already reported
	// it). An extra check would be unreachable by construction.

	return heapToSorted(h), nil
}

func (s *Store) validateQueryDim(embedding []float32) error {
	if len(embedding) == 0 {
		return fmt.Errorf("sqlite: query: %w", vectorstore.ErrEmptyEmbedding)
	}

	s.mu.RLock()
	dim := s.dim
	s.mu.RUnlock()

	if dim != 0 && len(embedding) != dim {
		// See Delete: pointer chain required for *DimensionMismatchError targets.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: len(embedding), Want: dim})
		return fmt.Errorf("sqlite: query: %w", mismatch)
	}

	return nil
}

func (s *Store) scanRows(ctx context.Context, rows *sql.Rows, embedding []float32, topK int) (*scoreHeap, error) {
	h := &scoreHeap{}
	seq := 0

	for rows.Next() {
		if seq >= maxScanRows {
			return nil, fmt.Errorf(
				"sqlite: query: table exceeds max scan rows %d "+
					"(use pgvector/qdrant for large corpora)",
				maxScanRows,
			)
		}

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: query: %w", err)
		}

		id, vec, meta, err := s.decodeRow(rows)
		if err != nil {
			return nil, err
		}

		score := vectorstore.CosineSimilarity(embedding, vec)
		s.pushHeap(h, topK, heaped{
			Seq:   seq,
			Score: float64(score),
			Match: vectorstore.ScoreMatch{ID: id, Score: score, Metadata: meta},
		})
		seq++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}

	return h, nil
}

func (s *Store) decodeRow(rows *sql.Rows) (string, []float32, map[string]any, error) {
	var id string

	var embBlob, metaBlob []byte

	if err := rows.Scan(&id, &embBlob, &metaBlob); err != nil {
		return "", nil, nil, fmt.Errorf("sqlite: scan: %w", err)
	}

	vec, err := decodeEmbedding(embBlob)
	if err != nil {
		return "", nil, nil, fmt.Errorf("sqlite: decode: %w", err)
	}

	meta, err := decodeMetadata(metaBlob)
	if err != nil {
		return "", nil, nil, fmt.Errorf("sqlite: query: metadata decode: %w", err)
	}

	return id, vec, meta, nil
}

func (s *Store) pushHeap(h *scoreHeap, topK int, item heaped) {
	if len(h.items) < topK {
		heap.Push(h, item)

		return
	}

	if item.Score > h.items[0].Score ||
		(item.Score == h.items[0].Score && item.Seq < h.items[0].Seq) {
		h.items[0] = item
		heap.Fix(h, 0)
	}
}

func heapToSorted(h *scoreHeap) []vectorstore.ScoreMatch {
	results := make([]vectorstore.ScoreMatch, 0, len(h.items))
	for len(h.items) > 0 {
		results = append(results, func() heaped { v, _ := heap.Pop(h).(heaped); return v }().Match)
	}

	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}

	return results
}

func (s *Store) checkAndSetDim(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dim == 0 {
		s.dim = n

		return nil
	}

	if n != s.dim {
		// See Delete: pointer chain required for *DimensionMismatchError targets.
		mismatch := error(&vectorstore.DimensionMismatchError{Got: n, Want: s.dim})
		return fmt.Errorf("sqlite: upsert: %w", mismatch)
	}

	return nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func encodeEmbedding(e []float32) []byte {
	b := make([]byte, 4*len(e))
	for i, v := range e {
		binary.LittleEndian.PutUint32(b[i*4:(i+1)*4], math.Float32bits(v))
	}

	return b
}

func decodeEmbedding(b []byte) ([]float32, error) {
	if len(b) == 0 {
		return nil, errors.New("empty embedding blob")
	}

	// Backwards compat: old rows were stored as JSON numbers. Binary blobs
	// may coincidentally start with '[' (0x5b), so only try JSON if the blob
	// is valid JSON. This avoids mis-decoding binary as JSON.
	if len(b) > 0 && b[0] == '[' && json.Valid(b) {
		if e, err := embeddingCodec.Decode(b); err == nil {
			return e, nil
		}
	}

	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding blob length %d", len(b))
	}

	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4 : (i+1)*4]))
	}

	return out, nil
}

func encodeMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}

	return metadataCodec.Encode(m)
}

func decodeMetadata(b []byte) (map[string]any, error) {
	if b == nil {
		return nil, nil //nolint:nilnil // verbatim port: nil metadata with nil error is the contract
	}

	m, err := metadataCodec.Decode(b)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// heaped is a scored candidate with an insertion sequence for stable tie-breaking.
type heaped struct {
	Seq   int
	Score float64
	Match vectorstore.ScoreMatch
}

// scoreHeap is a min-heap of heaped ordered by Score ascending, used to
// retain only the topK highest-similarity matches with bounded memory.
type scoreHeap struct {
	items []heaped
}

func (h *scoreHeap) Len() int { return len(h.items) }
func (h *scoreHeap) Less(i, j int) bool {
	if h.items[i].Score != h.items[j].Score {
		return h.items[i].Score < h.items[j].Score
	}

	return h.items[i].Seq > h.items[j].Seq
}
func (h *scoreHeap) Swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }

func (h *scoreHeap) Push(x any) {
	if v, ok := x.(heaped); ok {
		h.items = append(h.items, v)
	}
}

func (h *scoreHeap) Pop() any {
	old := h.items
	n := len(old)
	item := old[n-1]
	old[n-1] = heaped{}
	h.items = old[:n-1]

	return item
}

func validateDSN(dsn string) error {
	if strings.Contains(dsn, "\x00") || strings.Contains(dsn, "\n") || strings.Contains(dsn, "\r") {
		return errors.New("sqlite: dsn contains invalid control characters")
	}

	if strings.Contains(dsn, ";") {
		return errors.New("sqlite: dsn must not contain ';'")
	}

	return nil
}
