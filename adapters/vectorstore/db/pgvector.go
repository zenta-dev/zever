package db

import (
	"container/heap"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/codec"
	"github.com/zenta-dev/zever/shared/dbconn"
)

var (
	metadataCodec  = codec.JSONCodec[map[string]any]{}
	embeddingCodec = codec.JSONCodec[[]float32]{}
)

// vecRow is the vectors entity for the orm typed builder. Column order
// matches vecColumns.
type vecRow struct {
	ID        string
	Embedding []byte
	Metadata  []byte
}

// vecColumns is the entity column list.
var vecColumns = []string{"id", "embedding", "metadata"}

// driver is a DB-backed vectorstore.VectorStore. It is safe for concurrent
// use: the postgres dimension is fixed at construction, the sqlite
// dimension is guarded by mu, and Close is idempotent.
type driver struct {
	conn   coredb.DB
	tbl    orm.Table[vecRow]
	cID    orm.Column[vecRow, string]
	cEmb   orm.Column[vecRow, []byte]
	cMeta  orm.Column[vecRow, []byte]
	dim    int
	mu     sync.RWMutex
	owns   bool
	closed atomic.Bool
}

var _ vectorstore.VectorStore = (*driver)(nil)

// DefaultDDLTimeout bounds connect plus DDL during construction. Shared 10s
// floor with search/db: the pgvector ivfflat index build is slower
// than plain B-tree/GIN; single budget for connect+DDL during construction
// so they don't drift. Server backend; local embedded DB builds trivially
// inside it.
const DefaultDDLTimeout = 10 * time.Second

// maxScanRows caps the number of rows the sqlite leg scans brute-force.
// sqlite has no vector index, so Query is O(n). Without a cap a large table
// causes long latency and high memory pressure. Callers needing larger
// corpora should use pgvector on postgres or qdrant.
const maxScanRows = 10000

// New creates a DB-backed vectorstore.VectorStore. Empty DSN selects sqlite
// at ":memory:"; a postgres URL DSN opens postgres; any other non-empty DSN
// is a sqlite file path. The schema (vectors table plus the postgres
// extension/index) is created when missing. The driver owns its connection:
// Close releases it.
func New(o vectorstore.Options) (vectorstore.VectorStore, error) {
	return Open(o)
}

// Open creates a DB-backed vectorstore.VectorStore; see New.
func Open(o vectorstore.Options) (vectorstore.VectorStore, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector: %w", err)
	}

	dimension := o.Dimension
	if dimension <= 0 {
		dimension = vectorstore.DefaultDimension
	}

	poolOpts := dbconn.SplitDSN(o.DSN)

	var (
		conn coredb.DB
		err  error
	)

	if dbconn.IsPostgresDSN(o.DSN) {
		conn, err = dbpostgres.New(poolOpts)
	} else {
		conn, err = dbsqlite.New(poolOpts)
	}

	if err != nil {
		return nil, err
	}

	d, err := openFromDB(conn, o, dimension, true)
	if err != nil {
		_ = conn.Close(context.Background())

		return nil, err
	}

	return d, nil
}

// NewFromDB creates a DB-backed vectorstore.VectorStore over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned VectorStore does not close db, and a failed
// NewFromDB never closes db. Only sqlite and postgres dialects are
// supported; anything else fails closed. The configured dimension fixes the
// postgres leg; the sqlite leg learns its dimension from the first write
// and ignores it.
func NewFromDB(conn coredb.DB, o vectorstore.Options) (vectorstore.VectorStore, error) {
	if conn == nil {
		return nil, ErrNilDB
	}

	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector: %w", err)
	}

	dimension := o.Dimension
	if dimension <= 0 {
		dimension = vectorstore.DefaultDimension
	}

	return openFromDB(conn, o, dimension, false)
}

// OpenFromDB creates a DB-backed vectorstore.VectorStore over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o vectorstore.Options) (vectorstore.VectorStore, error) {
	return NewFromDB(conn, o)
}

// openFromDB pings conn, ensures the schema, and wires the driver. owns
// reports whether the driver owns conn and may close it in Close.
func openFromDB(conn coredb.DB, _ vectorstore.Options, dimension int, owns bool) (vectorstore.VectorStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDDLTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	const table = "vectors"

	d := &driver{
		conn:  conn,
		tbl:   orm.NewTable[vecRow](table, vecColumns),
		cID:   orm.NewColumn[vecRow, string](table, "id"),
		cEmb:  orm.NewColumn[vecRow, []byte](table, "embedding"),
		cMeta: orm.NewColumn[vecRow, []byte](table, "metadata"),
		owns:  owns,
	}

	if conn.Dialect() == "postgres" {
		d.dim = dimension
	}

	if err := d.ensureSchema(ctx, dimension); err != nil {
		return nil, err
	}

	return d, nil
}

// checkDialect fails closed unless the connection's dialect resolves to a
// VectorOpsDialect. SQLite reports SupportsVectorOps() == false yet stays
// admitted: its leg brute-forces cosine distance in Go instead of the
// pgvector extension, so presence of the capability set (not a true answer)
// is what admits a dialect here. An unresolvable dialect name, or one whose
// dialect reports no vector capability set at all, is rejected with the
// capability error, never a silently-degraded store.
func (d *driver) checkDialect() error {
	dd, err := dialect.For(d.conn.Dialect())
	if err != nil {
		return fmt.Errorf("pgvector: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}

	if _, ok := dd.(dialect.VectorOpsDialect); !ok {
		return fmt.Errorf("pgvector: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}

	return nil
}

// ensureSchema creates the vectors table plus the postgres extension/index
// when missing. DDL only: sqlite-leg writes go through the orm typed
// builder; the postgres leg needs raw SQL for the $n::vector/$n::jsonb
// casts (the typed builder cannot render casts) and both ranked reads use
// dialect-parameterized SELECTs (the orm query API cannot project the
// cosine score expression alongside rows).
func (d *driver) ensureSchema(ctx context.Context, dimension int) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if d.conn.Dialect() == "postgres" {
		if _, err := d.conn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			return fmt.Errorf("pgvector: create extension: %w", err)
		}

		existingDim, exists, err := d.embeddingColumnDim(ctx)
		if err != nil {
			return fmt.Errorf("pgvector: inspect embedding column: %w", err)
		}

		if exists {
			if err := checkDimension(existingDim, dimension); err != nil {
				return err
			}
		}

		if !exists {
			if _, err := d.conn.Exec(ctx, tableDDL(dimension)); err != nil {
				return fmt.Errorf("pgvector: create table: %w", err)
			}
		}

		if _, err := d.conn.Exec(ctx,
			`CREATE INDEX IF NOT EXISTS vectors_idx ON vectors USING ivfflat `+
				`(embedding vector_cosine_ops) WITH (lists = 100)`,
		); err != nil {
			return fmt.Errorf("pgvector: create index: %w", err)
		}

		return nil
	}

	if _, err := d.conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		embedding BLOB NOT NULL,
		metadata BLOB
	)`); err != nil {
		return fmt.Errorf("pgvector: create table: %w", err)
	}

	return nil
}

// embeddingColumnDim reports the dimension of the existing public.vectors
// embedding column. Postgres leg only.
func (d *driver) embeddingColumnDim(ctx context.Context) (int, bool, error) {
	rows, err := d.conn.Query(ctx, `SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relname = 'vectors'
		  AND a.attname = 'embedding'
		  AND a.attnum > 0`)
	if err != nil {
		return 0, false, err
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return 0, false, rowsErr
		}

		return 0, false, nil
	}

	var typ string
	if scanErr := rows.Scan(&typ); scanErr != nil {
		return 0, false, scanErr
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return 0, false, rowsErr
	}

	dim, err := parseVectorType(typ)
	if err != nil {
		return 0, false, err
	}

	return dim, true, nil
}

// checkDimension keeps its domain-specific message verbatim: the text carries
// both numbers greppably, and it wraps ErrDimensionMismatch for errors.Is.
func checkDimension(existing, configured int) error {
	if existing != configured {
		return fmt.Errorf("pgvector: existing vectors.embedding dimension %d does not match configured dimension %d: %w", existing, configured, vectorstore.ErrDimensionMismatch)
	}

	return nil
}

func parseVectorType(typ string) (int, error) {
	const prefix = "vector("
	if !strings.HasPrefix(typ, prefix) || !strings.HasSuffix(typ, ")") {
		return 0, fmt.Errorf("pgvector: unexpected embedding column type %q", typ)
	}

	dim, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(typ, prefix), ")"))
	if err != nil || dim <= 0 {
		return 0, fmt.Errorf("pgvector: unexpected embedding column type %q", typ)
	}

	return dim, nil
}

func tableDDL(dimension int) string {
	if dimension <= 0 {
		dimension = vectorstore.DefaultDimension
	}

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		embedding vector(%d) NOT NULL,
		metadata JSONB
	)`, dimension)
}

// upsertOne inserts or replaces vec. Batch callers loop it: the typed
// builder has no EXCLUDED reference, so one shared DO UPDATE SET list
// cannot carry per-row replacement values for a multi-row statement (same
// reason search/db loops IndexBatch).
func (d *driver) upsertOne(ctx context.Context, op string, vec vectorstore.Vector) error {
	if len(vec.Embedding) == 0 {
		return fmt.Errorf("pgvector: %s: %w", op, vectorstore.ErrEmptyEmbedding)
	}

	if d.conn.Dialect() == "postgres" {
		return d.upsertPostgres(ctx, op, vec)
	}

	if err := d.checkAndSetDim(len(vec.Embedding), op); err != nil {
		return err
	}

	blob := encodeEmbedding(vec.Embedding)

	metaBlob, err := encodeMetadata(vec.Metadata)
	if err != nil {
		return fmt.Errorf("pgvector: %s: encode metadata: %w", op, err)
	}

	err = orm.InsertInto(d.tbl).Values(
		orm.Set(d.cID, vec.ID),
		orm.Set(d.cEmb, blob),
		orm.Set(d.cMeta, metaBlob),
	).OnConflict(d.cID.Col()).DoUpdate(
		orm.Set(d.cEmb, blob),
		orm.Set(d.cMeta, metaBlob),
	).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("pgvector: %s: %w", op, err)
	}

	return nil
}

// upsertPostgres writes one row with explicit $n::vector/$n::jsonb casts.
// Embeddings use a JSON-string cast to avoid a pgvector-go dependency.
// Postgres leg only; the sqlite leg goes through the orm typed builder.
func (d *driver) upsertPostgres(ctx context.Context, op string, vec vectorstore.Vector) error {
	if len(vec.Embedding) != d.dim {
		// Pointer chain required: callers match with a *DimensionMismatchError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		mismatch := error(vectorstore.DimensionMismatchError{Got: len(vec.Embedding), Want: d.dim})
		return fmt.Errorf("pgvector: %s: %w (set the dimension option or recreate the vectors table)", op, mismatch)
	}

	metaJSON, err := metadataCodec.Encode(vec.Metadata)
	if err != nil {
		return fmt.Errorf("pgvector: %s: marshal metadata: %w", op, err)
	}

	vecJSON, err := embeddingCodec.Encode(vec.Embedding)
	if err != nil {
		return fmt.Errorf("pgvector: %s: marshal embedding: %w", op, err)
	}

	_, err = d.conn.Exec(ctx,
		`INSERT INTO vectors (id, embedding, metadata) VALUES ($1, $2::vector, $3::jsonb)
		 ON CONFLICT (id) DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata`,
		vec.ID, string(vecJSON), metaJSON,
	)
	if err != nil {
		return fmt.Errorf("pgvector: %s: %w", op, err)
	}

	return nil
}

// Upsert inserts or replaces a vector in the store.
func (d *driver) Upsert(ctx context.Context, vec vectorstore.Vector) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if err := vec.Validate(); err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	return d.upsertOne(ctx, "upsert", vec)
}

// UpsertBatch inserts or replaces all of vecs, one upsert per vector (see
// upsertOne). An empty vecs is a no-op.
func (d *driver) UpsertBatch(ctx context.Context, vecs []vectorstore.Vector) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	if len(vecs) == 0 {
		return nil
	}

	for i, vec := range vecs {
		if err := vec.Validate(); err != nil {
			return fmt.Errorf("pgvector: upsert batch: index %d: %w", i, err)
		}
	}

	for _, vec := range vecs {
		if err := d.upsertOne(ctx, "upsert batch", vec); err != nil {
			return err
		}
	}

	return nil
}

// Delete removes a vector by ID from the store. A missing id reports
// NotFound on both legs; the qdrant backend stays idempotent (see the
// VectorStore interface docs).
func (d *driver) Delete(ctx context.Context, id string) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	// RowsAffected replaces the earlier SELECT-then-DELETE two-round-trip
	// shape with a single statement: zero affected rows means the vector
	// does not exist.
	n, err := orm.DeleteFrom(d.tbl).Where(d.cID.Eq(id)).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("pgvector: delete: %w", err)
	}

	if n == 0 {
		// Pointer chain required: callers match with a *NotFoundError
		// target. The error-typed intermediate keeps that chain while
		// satisfying govet's printf check.
		notFound := error(vectorstore.NotFoundError{ID: id})
		return fmt.Errorf("pgvector: delete: %w", notFound)
	}

	return nil
}

// Query finds the top-K nearest vectors to the given embedding.
func (d *driver) Query(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if err := d.checkDialect(); err != nil {
		return nil, err
	}

	if len(embedding) == 0 {
		return nil, fmt.Errorf("pgvector: query: %w", vectorstore.ErrEmptyEmbedding)
	}

	if topK <= 0 {
		topK = vectorstore.DefaultTopK
	}

	if d.conn.Dialect() == "postgres" {
		return d.queryPostgres(ctx, embedding, topK)
	}

	return d.querySQLite(ctx, embedding, topK)
}

// queryPostgres runs the pgvector cosine query. Scores are 1 minus cosine
// distance (higher means more similar), ordered nearest-first.
func (d *driver) queryPostgres(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if len(embedding) != d.dim {
		// See upsertPostgres: pointer chain required for
		// *DimensionMismatchError targets.
		mismatch := error(vectorstore.DimensionMismatchError{Got: len(embedding), Want: d.dim})
		return nil, fmt.Errorf("pgvector: query: %w (set the dimension option or recreate the vectors table)", mismatch)
	}

	vecJSON, err := embeddingCodec.Encode(embedding)
	if err != nil {
		return nil, fmt.Errorf("pgvector: query: marshal embedding: %w", err)
	}

	vecText := string(vecJSON)

	rows, err := d.conn.Query(ctx,
		`SELECT id, 1 - (embedding <=> $1::vector) AS score, metadata
		 FROM vectors
		 ORDER BY embedding <=> $2::vector
		 LIMIT $3`,
		vecText, vecText, topK,
	)
	if err != nil {
		return nil, fmt.Errorf("pgvector: query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var results []vectorstore.ScoreMatch

	for rows.Next() {
		var m vectorstore.ScoreMatch

		// Postgres decodes float8 into float64, so scan into float64 then narrow.
		var score float64

		var metaRaw any

		if err := rows.Scan(&m.ID, &score, &metaRaw); err != nil {
			return nil, fmt.Errorf("pgvector: scan: %w", err)
		}

		m.Score = float32(score)

		meta, err := decodeHitMeta(metaRaw)
		if err != nil {
			return nil, fmt.Errorf("pgvector: metadata decode: %w", err)
		}

		m.Metadata = meta

		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: rows: %w", err)
	}

	return results, nil
}

// querySQLite is the brute-force fallback without a vector index: it scans
// every row and keeps only topK candidates in a bounded heap (O(topK)
// memory). For large corpora use postgres/qdrant. To avoid OOM/latency on
// accidental large tables, the scan is capped at maxScanRows and returns an
// error if exceeded. Ranking never silently misranks: every stored row is
// scored with vectorstore.CosineSimilarity, ties break by insertion order,
// and an oversized table fails closed instead of returning a partial topK.
func (d *driver) querySQLite(ctx context.Context, embedding []float32, topK int) ([]vectorstore.ScoreMatch, error) {
	if err := d.validateQueryDim(embedding); err != nil {
		return nil, err
	}

	rows, err := d.conn.Query(ctx, `SELECT id, embedding, metadata FROM vectors`)
	if err != nil {
		return nil, fmt.Errorf("pgvector: query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	h, err := d.scanRows(ctx, rows, embedding, topK)
	if err != nil {
		return nil, err
	}

	return heapToSorted(h)
}

// decodeHitMeta decodes one match's stored metadata JSON. Postgres cells
// arrive as []byte (JSONB) or string.
func decodeHitMeta(v any) (map[string]any, error) {
	var raw []byte

	switch t := v.(type) {
	case nil:
		return nil, nil //nolint:nilnil // nil metadata with nil error is the contract
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		return nil, fmt.Errorf("pgvector: unsupported metadata %T: %w", v, vectorstore.ErrInvalidMetadata)
	}

	if raw == nil {
		return nil, nil //nolint:nilnil // nil metadata with nil error is the contract
	}

	m, err := metadataCodec.Decode(raw)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (d *driver) validateQueryDim(embedding []float32) error {
	d.mu.RLock()
	dim := d.dim
	d.mu.RUnlock()

	if dim != 0 && len(embedding) != dim {
		// See upsertPostgres: pointer chain required for
		// *DimensionMismatchError targets.
		mismatch := error(vectorstore.DimensionMismatchError{Got: len(embedding), Want: dim})
		return fmt.Errorf("pgvector: query: %w", mismatch)
	}

	return nil
}

func (d *driver) scanRows(ctx context.Context, rows coredb.Rows, embedding []float32, topK int) (*scoreHeap, error) {
	// Preallocate for the rows this scan can actually retain. capHint is
	// clamped to maxScanRows so the allocation never depends on the
	// caller-supplied topK, which is otherwise unbounded.
	capHint := topK
	if capHint > maxScanRows {
		capHint = maxScanRows
	}

	h := &scoreHeap{items: make([]heaped, 0, capHint)}
	seq := 0

	for rows.Next() {
		if seq >= maxScanRows {
			return nil, fmt.Errorf(
				"pgvector: query: table exceeds max scan rows %d "+
					"(use pgvector on postgres or qdrant for large corpora)",
				maxScanRows,
			)
		}

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("pgvector: query: %w", err)
		}

		id, vec, metaBlob, err := decodeRow(rows)
		if err != nil {
			return nil, err
		}

		score := vectorstore.CosineSimilarity(embedding, vec)
		pushHeap(h, topK, heaped{
			Seq:     seq,
			Score:   float64(score),
			Match:   vectorstore.ScoreMatch{ID: id, Score: score},
			metaRaw: metaBlob,
		})
		seq++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: query: %w", err)
	}

	return h, nil
}

func decodeRow(rows coredb.Rows) (string, []float32, []byte, error) {
	var id string

	var embBlob, metaBlob []byte

	if err := rows.Scan(&id, &embBlob, &metaBlob); err != nil {
		return "", nil, nil, fmt.Errorf("pgvector: scan: %w", err)
	}

	vec, err := decodeEmbedding(embBlob)
	if err != nil {
		return "", nil, nil, fmt.Errorf("pgvector: decode: %w", err)
	}

	return id, vec, metaBlob, nil
}

func pushHeap(h *scoreHeap, topK int, item heaped) {
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

// heapToSorted drains the heap into score order and decodes metadata for the
// survivors only: rows pushed out of the topK never pay the JSON decode, so
// corrupt metadata in a non-surviving row no longer fails the query.
func heapToSorted(h *scoreHeap) ([]vectorstore.ScoreMatch, error) {
	results := make([]vectorstore.ScoreMatch, 0, len(h.items))
	for len(h.items) > 0 {
		v, _ := heap.Pop(h).(heaped)

		meta, err := decodeMetadata(v.metaRaw)
		if err != nil {
			return nil, fmt.Errorf("pgvector: query: metadata decode: %w", err)
		}

		v.Match.Metadata = meta
		results = append(results, v.Match)
	}

	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}

	return results, nil
}

func (d *driver) checkAndSetDim(n int, op string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dim == 0 {
		d.dim = n

		return nil
	}

	if n != d.dim {
		// See upsertPostgres: pointer chain required for
		// *DimensionMismatchError targets.
		mismatch := error(vectorstore.DimensionMismatchError{Got: n, Want: d.dim})
		return fmt.Errorf("pgvector: %s: %w", op, mismatch)
	}

	return nil
}

// Close releases the database pool when this driver owns its connection
// (built via New/Open). Drivers built via NewFromDB/OpenFromDB borrow the
// caller's DB and Close is a no-op. Close is idempotent.
func (d *driver) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if !d.owns {
		return nil
	}

	return d.conn.Close(context.Background())
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
		return nil, fmt.Errorf("pgvector: empty embedding blob: %w", vectorstore.ErrEmptyEmbedding)
	}

	// Backwards compat: rows written by adapters/vectorstore/sqlite were
	// stored as JSON numbers. Binary blobs may coincidentally start with
	// '[' (0x5b), so only try JSON if the blob is valid JSON. This avoids
	// mis-decoding binary as JSON.
	if len(b) > 0 && b[0] == '[' && json.Valid(b) {
		if e, err := embeddingCodec.Decode(b); err == nil {
			return e, nil
		}
	}

	if len(b)%4 != 0 {
		return nil, fmt.Errorf("pgvector: invalid embedding blob length %d: %w", len(b), ErrInvalidEmbedding)
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
		return nil, nil //nolint:nilnil // nil metadata with nil error is the contract
	}

	m, err := metadataCodec.Decode(b)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// heaped is a scored candidate with an insertion sequence for stable
// tie-breaking. metaRaw carries the undecoded metadata blob: only topK
// survivors get it decoded in heapToSorted.
type heaped struct {
	Seq     int
	Score   float64
	Match   vectorstore.ScoreMatch
	metaRaw []byte
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
