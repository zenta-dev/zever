package db

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// mustFileStore opens a file-backed store. File-backed (not ":memory:") keeps
// parallel tests isolated: ":memory:" is shared-cache, so concurrent tests
// would see each other's rows and race on the learned dimension.
func mustFileStore(t *testing.T) vectorstore.VectorStore {
	t.Helper()

	s, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

// TestSQLiteLeg_concurrentUpsertQuery exercises upserts and brute-force
// queries from many goroutines against one store; the race detector enforces
// the dimension-lock and connection-pool correctness.
func TestSQLiteLeg_concurrentUpsertQuery(t *testing.T) {
	t.Parallel()

	s, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()
	emb := []float32{0.1, 0.2, 0.3, 0.4}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			vec := vectorstore.Vector{ID: "c-" + strconv.Itoa(i), Embedding: emb}

			if uerr := s.Upsert(ctx, vec); uerr != nil {
				t.Errorf("Upsert err = %v, want nil", uerr)
				return
			}

			if _, qerr := s.Query(ctx, emb, 5); qerr != nil {
				t.Errorf("Query err = %v, want nil", qerr)
			}
		}(i)
	}

	wg.Wait()
}

// TestEdgeQuery_corruptMetadataBelowTopK documents the lazy metadata decode:
// metadata JSON is parsed only for rows that survive the topK heap. A row
// with corrupt metadata that ranks below topK is discarded before its
// metadata is decoded, so the query succeeds; the same row fails the query
// once topK admits it.
func TestEdgeQuery_corruptMetadataBelowTopK(t *testing.T) {
	t.Parallel()

	// File-backed, not :memory: :memory: is shared-cache, so parallel
	// tests would see each other's rows and perturb the ranking.
	s, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "vectors.db")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	query := []float32{1, 0, 0, 0}

	for i := 0; i < 5; i++ {
		mustUpsert(t, s, vectorstore.Vector{
			ID:        "hit-" + strconv.Itoa(i),
			Embedding: query,
			Metadata:  map[string]any{"i": i},
		})
	}

	// Orthogonal embedding scores 0, below every hit; its corrupt metadata
	// must never be decoded while topK excludes it.
	if _, err = d.conn.Exec(ctx,
		`INSERT INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		"corrupt", encodeEmbedding([]float32{0, 1, 0, 0}), []byte("{not json"),
	); err != nil {
		t.Fatalf("insert corrupt row: %v", err)
	}

	hits, err := s.Query(ctx, query, 5)

	if err != nil {
		t.Fatalf("Query(topK=5) err = %v, want nil (corrupt row below topK)", err)
	}

	if len(hits) != 5 {
		t.Fatalf("Query(topK=5) = %d hits, want 5", len(hits))
	}

	_, err = s.Query(ctx, query, 6)
	if err == nil {
		t.Fatal("Query(topK=6) = nil, want metadata decode error")
	}

	if !strings.Contains(err.Error(), "metadata decode") {
		t.Fatalf("Query(topK=6) err = %v, want metadata decode error", err)
	}
}

// TestEdgeDecodeEmbedding_JSONPrefixInvalidJSON proves the legacy-JSON sniff
// only engages on valid JSON: a blob starting with '[' that is not valid JSON
// falls through to the binary path and fails on its length, never mis-decoding
// text as floats.
func TestEdgeDecodeEmbedding_JSONPrefixInvalidJSON(t *testing.T) {
	t.Parallel()

	// 9 bytes: not valid JSON, not a multiple of 4.
	_, err := decodeEmbedding([]byte("[not jsonx"))
	if !errors.Is(err, ErrInvalidEmbedding) {
		t.Fatalf("decodeEmbedding(\"[not jsonx\") err = %v, want ErrInvalidEmbedding", err)
	}
}

// TestEdgeDecodeMetadata_malformed rejects corrupt JSON and maps nil input to
// nil metadata with no error.
func TestEdgeDecodeMetadata_malformed(t *testing.T) {
	t.Parallel()

	if got, err := decodeMetadata(nil); err != nil || got != nil {
		t.Fatalf("decodeMetadata(nil) = %v, %v; want nil, nil", got, err)
	}

	if _, err := decodeMetadata([]byte("{not json")); err == nil {
		t.Fatal("decodeMetadata(corrupt) = nil, want error")
	}
}

// TestEdgeEncodeMetadata_nil encodes nil metadata to nil so callers skip the
// sidecar write entirely.
func TestEdgeEncodeMetadata_nil(t *testing.T) {
	t.Parallel()

	b, err := encodeMetadata(nil)
	if err != nil || b != nil {
		t.Fatalf("encodeMetadata(nil) = %v, %v; want nil, nil", b, err)
	}
}

// TestEdgeUpsertBatch_empty: an empty or nil batch is a no-op.
func TestEdgeUpsertBatch_empty(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	if err := s.UpsertBatch(t.Context(), nil); err != nil {
		t.Fatalf("UpsertBatch(nil) err = %v, want nil", err)
	}

	if err := s.UpsertBatch(t.Context(), []vectorstore.Vector{}); err != nil {
		t.Fatalf("UpsertBatch(empty) err = %v, want nil", err)
	}
}

// TestEdgeUpsertBatch_invalidVecAtIndex names the offending index and wraps
// ErrInvalidMetadata. File-backed: ":memory:" is shared-cache, so parallel
// tests would race on the learned dimension.
func TestEdgeUpsertBatch_invalidVecAtIndex(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)

	vecs := []vectorstore.Vector{
		{ID: "ok", Embedding: []float32{1, 0}},
		{ID: "bad", Embedding: []float32{1, 0}, Metadata: map[string]any{"ch": make(chan int)}},
	}

	err := s.UpsertBatch(t.Context(), vecs)
	if err == nil {
		t.Fatal("UpsertBatch() = nil, want error")
	}

	if !strings.Contains(err.Error(), "index 1") {
		t.Fatalf("UpsertBatch() err = %v, want index 1", err)
	}

	if !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Fatalf("UpsertBatch() err = %v, want ErrInvalidMetadata", err)
	}
}

// TestEdgeUpsert_invalidMetadata rejects unencodable metadata on the single
// upsert path. File-backed: see TestEdgeUpsertBatch_invalidVecAtIndex.
func TestEdgeUpsert_invalidMetadata(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)

	err := s.Upsert(t.Context(), vectorstore.Vector{
		ID:        "bad",
		Embedding: []float32{1, 0},
		Metadata:  map[string]any{"ch": make(chan int)},
	})
	if !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Fatalf("Upsert() err = %v, want ErrInvalidMetadata", err)
	}
}

// TestEdgeQuery_topKDefault: topK <= 0 falls back to DefaultTopK.
func TestEdgeQuery_topKDefault(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)
	ctx := t.Context()

	for i := 0; i < vectorstore.DefaultTopK+2; i++ {
		mustUpsert(t, s, vectorstore.Vector{
			ID:        "doc-" + strconv.Itoa(i),
			Embedding: []float32{1, 0, 0, 0},
		})
	}

	for _, topK := range []int{0, -1} {
		hits, err := s.Query(ctx, []float32{1, 0, 0, 0}, topK)
		if err != nil {
			t.Fatalf("Query(topK=%d) err = %v, want nil", topK, err)
		}

		if len(hits) != vectorstore.DefaultTopK {
			t.Fatalf("Query(topK=%d) = %d hits, want %d", topK, len(hits), vectorstore.DefaultTopK)
		}
	}
}

// TestEdgeQuery_dimensionMismatchSQLite: a query embedding whose length
// differs from the stored dimension fails closed. File-backed: see
// TestEdgeUpsertBatch_invalidVecAtIndex.
func TestEdgeQuery_dimensionMismatchSQLite(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)
	ctx := t.Context()

	mustUpsert(t, s, vectorstore.Vector{ID: "d", Embedding: []float32{1, 0, 0}})

	_, err := s.Query(ctx, []float32{1, 0}, 5)

	var mmErr vectorstore.DimensionMismatchError
	if !errors.As(err, &mmErr) {
		t.Fatalf("Query(wrong dim) err = %T %v, want *DimensionMismatchError", err, err)
	}

	if mmErr.Got != 2 || mmErr.Want != 3 {
		t.Errorf("DimensionMismatchError = got %d want %d, want got 2 want 3", mmErr.Got, mmErr.Want)
	}
}

// TestEdgeQuery_emptyStore: querying an empty store returns no hits, no error.
// File-backed: ":memory:" is shared-cache, so parallel tests would see each
// other's rows.
func TestEdgeQuery_emptyStore(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)

	hits, err := s.Query(t.Context(), []float32{1, 0}, 5)
	if err != nil {
		t.Fatalf("Query() err = %v, want nil", err)
	}

	if len(hits) != 0 {
		t.Fatalf("Query() = %d hits, want 0", len(hits))
	}
}

// TestEdgeDelete_emptyID: deleting an empty id matches nothing and reports
// NotFound.
func TestEdgeDelete_emptyID(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	if err := s.Delete(t.Context(), ""); !errors.Is(err, vectorstore.ErrNotFound) {
		t.Fatalf("Delete(\"\") err = %v, want ErrNotFound", err)
	}
}

// TestEdgeClose_idempotentOwned: Close on an owned driver is idempotent.
func TestEdgeClose_idempotentOwned(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() second err = %v, want nil", err)
	}
}

// TestEdgeNewFromDB_invalidOptions: options are validated before the dialect
// check.
func TestEdgeNewFromDB_invalidOptions(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	if _, err := NewFromDB(conn, vectorstore.Options{Dimension: -1}); err == nil {
		t.Fatal("NewFromDB(Dimension -1) = nil, want error")
	}
}

// TestEdgeTableDDL_defaultDimension: a non-positive dimension falls back to
// DefaultDimension in the postgres DDL.
func TestEdgeTableDDL_defaultDimension(t *testing.T) {
	t.Parallel()

	for _, dim := range []int{0, -5} {
		if ddl := tableDDL(dim); !strings.Contains(ddl, "vector(1536)") {
			t.Fatalf("tableDDL(%d) = %q, want vector(1536)", dim, ddl)
		}
	}
}

// TestEdgeScanRows_maxScanRowsCap: a table beyond maxScanRows fails closed with
// a scan-cap error instead of returning a partial topK.
func TestEdgeScanRows_maxScanRowsCap(t *testing.T) {
	t.Parallel()

	s := mustFileStore(t)

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	ctx := t.Context()

	// One multi-row INSERT seeds maxScanRows+1 rows.
	var sb strings.Builder

	sb.WriteString("INSERT INTO vectors (id, embedding, metadata) VALUES ")

	for i := 0; i <= maxScanRows; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}

		sb.WriteString("('")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("', X'")
		sb.WriteString(hex.EncodeToString(encodeEmbedding([]float32{float32(i), 0, 0, 0})))
		sb.WriteString("', NULL)")
	}

	if _, err := d.conn.Exec(ctx, sb.String()); err != nil {
		t.Fatalf("seed insert err = %v", err)
	}

	_, err := s.Query(ctx, []float32{1, 0, 0, 0}, 5)
	if err == nil {
		t.Fatal("Query() = nil, want max scan rows error")
	}

	if !strings.Contains(err.Error(), "max scan rows") {
		t.Fatalf("Query() err = %v, want max scan rows", err)
	}
}
