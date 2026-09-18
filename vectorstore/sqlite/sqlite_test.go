package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/vectorstore"
)

func newMemoryStore(t *testing.T) *Store {
	t.Helper()

	vs, err := New(vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	s, ok := vs.(*Store)
	if !ok {
		t.Fatalf("New() returned %T, want *Store", vs)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestSQLiteRoundtrip(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{
		ID:        "v1",
		Embedding: []float32{1, 0, 0},
		Metadata:  map[string]any{"name": "alpha"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{
		ID:        "v2",
		Embedding: []float32{0, 1, 0},
		Metadata:  map[string]any{"name": "beta"},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	if results[0].ID != "v1" {
		t.Fatalf("expected v1, got %s", results[0].ID)
	}

	if results[0].Score < 0.99 {
		t.Fatalf("expected high score, got %f", results[0].Score)
	}

	if results[0].Metadata["name"] != "alpha" {
		t.Fatalf("expected metadata alpha, got %v", results[0].Metadata)
	}

	if delErr := s.Delete(ctx, "v1"); delErr != nil {
		t.Fatal(delErr)
	}

	after, err := s.Query(ctx, []float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(after) != 1 || after[0].ID != "v2" {
		t.Fatalf("expected v2 after delete, got %v", after)
	}
}

func TestSQLiteFileBacked(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "vec.db")
	ctx := context.Background()

	s, err := New(vectorstore.Options{DSN: path})
	if err != nil {
		t.Fatal(err)
	}

	if upErr := s.Upsert(ctx, vectorstore.Vector{ID: "f1", Embedding: []float32{1, 0}}); upErr != nil {
		_ = s.Close()
		t.Fatal(upErr)
	}

	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	s2, err := New(vectorstore.Options{DSN: path})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = s2.Close() }()

	results, err := s2.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 || results[0].ID != "f1" {
		t.Fatalf("expected persisted f1, got %v", results)
	}
}

func TestSQLiteDeleteNotFound(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	err := s.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}

	if !errors.Is(err, vectorstore.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	var nf *vectorstore.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %T is not *NotFoundError", err)
	}

	if nf.ID != "nonexistent" {
		t.Fatalf("carried ID = %q, want %q", nf.ID, "nonexistent")
	}
}

func TestSQLiteUpsertEmptyEmbeddingRejected(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1"}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("expected ErrEmptyEmbedding, got %v", err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v0", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v2", Embedding: []float32{}}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("expected ErrEmptyEmbedding after populated, got %v", err)
	}
}

func TestSQLiteQueryEmptyEmbeddingRejected(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.Query(ctx, nil, 5); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("expected ErrEmptyEmbedding, got %v", err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Query(ctx, []float32{}, 5); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("expected ErrEmptyEmbedding after populated, got %v", err)
	}
}

func TestSQLiteUpsertOverwrite(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{0, 1}}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, []float32{0, 1}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 || results[0].ID != "v1" {
		t.Fatal("expected overwritten vector")
	}

	if results[0].Score < 0.99 {
		t.Fatalf("expected high score after overwrite, got %f", results[0].Score)
	}
}

func TestSQLiteQueryEmpty(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	results, err := s.Query(context.Background(), []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 0 {
		t.Fatalf("expected empty results, got %v", results)
	}
}

func TestSQLiteQueryTopKZeroDefaults(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		vec := make([]float32, 3)
		vec[i%3] = 1

		if err := s.Upsert(ctx, vectorstore.Vector{ID: fmt.Sprintf("z%02d", i), Embedding: vec}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.Query(ctx, []float32{1, 0, 0}, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != vectorstore.DefaultTopK {
		t.Fatalf("expected %d results, got %d", vectorstore.DefaultTopK, len(results))
	}

	if results[0].Score < 0.99 {
		t.Fatalf("expected best match first, got %s score %f", results[0].ID, results[0].Score)
	}
}

func TestSQLiteQueryTopKNegativeDefaults(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		vec := make([]float32, 3)
		vec[i%3] = 1

		if err := s.Upsert(ctx, vectorstore.Vector{ID: fmt.Sprintf("n%02d", i), Embedding: vec}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.Query(ctx, []float32{1, 0, 0}, -1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != vectorstore.DefaultTopK {
		t.Fatalf("expected %d results, got %d", vectorstore.DefaultTopK, len(results))
	}
}

func TestSQLiteQueryTopK(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		vec := make([]float32, 3)
		vec[i%3] = 1

		if err := s.Upsert(ctx, vectorstore.Vector{ID: fmt.Sprintf("k%02d", i), Embedding: vec}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.Query(ctx, []float32{1, 0, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestSQLiteMetadataNil(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 || results[0].Metadata != nil {
		t.Fatalf("expected nil metadata, got %v", results)
	}
}

func TestSQLiteMetadataRoundtrip(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	meta := map[string]any{"name": "alpha", "n": float64(3)}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}, Metadata: meta}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %v", results)
	}

	if results[0].Metadata["name"] != "alpha" {
		t.Fatalf("expected metadata alpha, got %v", results[0].Metadata)
	}
}

func TestSQLiteMixedDimensionUpsertRejected(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0, 0}}); err != nil {
		t.Fatal(err)
	}

	err := s.Upsert(ctx, vectorstore.Vector{ID: "v2", Embedding: []float32{1, 0}})
	if err == nil {
		t.Fatal("expected error for mismatched dimension")
	}

	if !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}

	var dm *vectorstore.DimensionMismatchError
	if !errors.As(err, &dm) {
		t.Fatalf("err %T is not *DimensionMismatchError", err)
	}

	if dm.Got != 2 || dm.Want != 3 {
		t.Fatalf("Got/Want = %d/%d, want 2/3", dm.Got, dm.Want)
	}
}

func TestSQLiteMixedDimensionQueryRejected(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0, 0}}); err != nil {
		t.Fatal(err)
	}

	_, err := s.Query(ctx, []float32{1, 0}, 1)
	if err == nil {
		t.Fatal("expected error for mismatched query dimension")
	}

	if !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}

	var dm *vectorstore.DimensionMismatchError
	if !errors.As(err, &dm) {
		t.Fatalf("err %T is not *DimensionMismatchError", err)
	}

	if dm.Got != 2 || dm.Want != 3 {
		t.Fatalf("Got/Want = %d/%d, want 2/3", dm.Got, dm.Want)
	}
}

func TestSQLiteCorruptMetadataErrors(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `INSERT INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		"v1", []byte(`[1,0,0]`), []byte(`{oops`)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Query(ctx, []float32{1, 0, 0}, 1); err == nil {
		t.Fatal("expected error for corrupt metadata")
	}
}

func TestSQLiteCorruptEmbeddingErrors(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	// Length 3: not valid JSON, not a multiple of 4.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		"bad", []byte{1, 2, 3}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Query(ctx, []float32{1, 0, 0}, 1); err == nil {
		t.Fatal("expected error for corrupt embedding")
	}
}

func TestSQLiteEmptyEmbeddingBlobErrors(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `INSERT INTO vectors (id, embedding, metadata) VALUES (?, ?, ?)`,
		"empty", []byte{}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Query(ctx, []float32{1, 0}, 1); err == nil {
		t.Fatal("expected error for empty embedding blob")
	}
}

func TestSQLiteQueryTopKBoundedOrdering(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	// Insert far more rows than the requested topK. Query vector is the unit
	// vector along the x-axis, so a stored vector v = [cos, sin] has cosine
	// similarity exactly cos. Use strictly decreasing cos so the topK results
	// are unambiguously the first topK ids.
	const rows = 5000

	for i := 0; i < rows; i++ {
		cos := math.Cos(math.Pi / 2 * float64(i+1) / float64(rows+1))
		sin := math.Sin(math.Pi / 2 * float64(i+1) / float64(rows+1))

		if err := s.Upsert(ctx, vectorstore.Vector{
			ID:        fmt.Sprintf("r%05d", i),
			Embedding: []float32{float32(cos), float32(sin)},
			Metadata:  map[string]any{"idx": i},
		}); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
	}

	const topK = 5

	results, err := s.Query(ctx, []float32{1, 0}, topK)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != topK {
		t.Fatalf("expected %d results, got %d", topK, len(results))
	}

	// The true topK are the rows with the smallest angle (highest cos).
	for i, r := range results {
		if r.ID != fmt.Sprintf("r%05d", i) {
			t.Fatalf("results[%d]: expected top-%d row r%05d, got %s", i, topK, i, r.ID)
		}

		if i < topK-1 && r.Score < results[i+1].Score {
			t.Fatalf("results not sorted descending at %d: %f < %f", i, r.Score, results[i+1].Score)
		}
	}
}

func TestSQLiteScanCapExceeded(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	for i := 0; i < maxScanRows+1; i++ {
		if err := s.Upsert(ctx, vectorstore.Vector{
			ID:        fmt.Sprintf("c%05d", i),
			Embedding: []float32{float32(i%10 + 1), float32((i*2)%10 + 1)},
		}); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
	}

	_, err := s.Query(ctx, []float32{1, 1}, 5)
	if err == nil {
		t.Fatal("expected error when exceeding maxScanRows, got nil")
	}

	if !strings.Contains(err.Error(), "max scan rows") {
		t.Fatalf("expected max scan rows error, got %v", err)
	}
}

func TestSQLiteScanCapBoundary(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	for i := 0; i < maxScanRows; i++ {
		if err := s.Upsert(ctx, vectorstore.Vector{
			ID:        fmt.Sprintf("b%05d", i),
			Embedding: []float32{float32(i%10 + 1), float32((i*2)%10 + 1)},
		}); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
	}

	// Exactly maxScanRows must still succeed and stay bounded to topK heap.
	results, err := s.Query(ctx, []float32{1, 1}, 5)
	if err != nil {
		t.Fatalf("query at cap: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

func TestSQLiteConcurrent(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	const n = 32

	var wg sync.WaitGroup

	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs <- s.Upsert(ctx, vectorstore.Vector{
				ID:        fmt.Sprintf("v%d", i),
				Embedding: []float32{float32(i + 1), 0, 0},
			})
		}(i)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}

	results, err := s.Query(ctx, []float32{1, 0, 0}, n)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	vs, err := New(vectorstore.Options{})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = vs.Close() }()

	ctx := context.Background()

	if upErr := vs.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); upErr != nil {
		t.Fatal(upErr)
	}

	results, err := vs.Query(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 || results[0].ID != "v1" {
		t.Fatalf("expected v1, got %v", results)
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	vs, err := New(vectorstore.Options{Dimension: -1})
	if err == nil {
		t.Fatal("expected invalid options error, got nil")
	}

	if !errors.Is(err, vectorstore.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}

	if vs != nil {
		t.Fatal("expected nil store on invalid options")
	}
}

func TestNewBadDirDSN(t *testing.T) {
	t.Parallel()

	vs, err := New(vectorstore.Options{DSN: filepath.Join(t.TempDir(), "no-such-dir", "vec.db")})
	if err == nil {
		t.Fatal("expected error for bad-dir DSN, got nil")
	}

	if vs != nil {
		t.Fatal("expected nil store on bad-dir DSN")
	}
}

func TestNewBadDSNControlChars(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"a\x00b", "a\nb", "a\rb", "a;b"} {
		vs, err := New(vectorstore.Options{DSN: dsn})
		if err == nil {
			t.Fatalf("expected error for DSN %q, got nil", dsn)
		}

		if vs != nil {
			t.Fatalf("expected nil store for DSN %q", dsn)
		}
	}
}

func TestSQLiteDeleteExecError(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.ExecContext(ctx,
		`CREATE TRIGGER no_del BEFORE DELETE ON vectors BEGIN SELECT RAISE(ABORT, 'delete blocked'); END`); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, "v1"); err == nil {
		t.Fatal("expected error when DELETE fails")
	}
}

func TestSQLiteClose(t *testing.T) {
	t.Parallel()

	s, err := New(vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := s.Query(context.Background(), []float32{1, 0}, 1); err == nil {
		t.Fatal("expected error querying closed store")
	}

	if err := s.Upsert(context.Background(), vectorstore.Vector{ID: "v", Embedding: []float32{1, 0}}); err == nil {
		t.Fatal("expected error upserting closed store")
	}

	if err := s.Delete(context.Background(), "v"); err == nil {
		t.Fatal("expected error deleting from closed store")
	}
}

func TestSQLiteMemoryIsolation(t *testing.T) {
	t.Parallel()

	a := newMemoryStore(t)
	b := newMemoryStore(t)
	ctx := context.Background()

	if err := a.Upsert(ctx, vectorstore.Vector{ID: "only-a", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	results, err := b.Query(ctx, []float32{1, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 0 {
		t.Fatalf("expected isolated empty store, got %v", results)
	}
}

func TestValidateDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dsn     string
		wantErr bool
	}{
		{"", false},
		{":memory:", false},
		{"file.db", false},
		{"/tmp/vec.db", false},
		{"a\x00b", true},
		{"a\nb", true},
		{"a\rb", true},
		{"a;b", true},
		{"a;SELECT 1", true},
	}

	for _, tc := range cases {
		if err := validateDSN(tc.dsn); (err != nil) != tc.wantErr {
			t.Errorf("validateDSN(%q) err = %v, wantErr %v", tc.dsn, err, tc.wantErr)
		}
	}
}

func TestDecodeEmbeddingJSONLegacy(t *testing.T) {
	t.Parallel()

	vec, err := decodeEmbedding([]byte(`[1,0,0]`))
	if err != nil {
		t.Fatal(err)
	}

	if len(vec) != 3 || vec[0] != 1 || vec[1] != 0 || vec[2] != 0 {
		t.Fatalf("unexpected legacy decode: %v", vec)
	}
}

func TestDecodeEmbeddingBinaryStartingWithBracket(t *testing.T) {
	t.Parallel()

	// Find a float32 whose little-endian encoding starts with '[' (0x5b)
	// but is not valid JSON, proving the binary path is taken.
	var first float32

	found := false

	for bits := uint32(0x3F00005B); bits < 0x42000000; bits++ {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], bits)

		if b[0] == '[' && !json.Valid(b[:]) {
			first = math.Float32frombits(bits)
			found = true

			break
		}
	}

	if !found {
		t.Fatal("could not find binary float starting with 0x5b")
	}

	blob := encodeEmbedding([]float32{first, 0})

	if len(blob) == 0 || blob[0] != '[' {
		t.Fatalf("test setup broken: blob does not start with '[': %v", blob)
	}

	vec, err := decodeEmbedding(blob)
	if err != nil {
		t.Fatal(err)
	}

	if len(vec) != 2 || vec[0] != first || vec[1] != 0 {
		t.Fatalf("binary guard mis-decoded blob as JSON: %v", vec)
	}
}

func TestDecodeEmbeddingErrors(t *testing.T) {
	t.Parallel()

	if _, err := decodeEmbedding(nil); err == nil {
		t.Fatal("expected error for empty blob")
	}

	if _, err := decodeEmbedding([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for bad-length blob")
	}
}

func TestDecodeMetadataNil(t *testing.T) {
	t.Parallel()

	m, err := decodeMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}

	if m != nil {
		t.Fatalf("expected nil metadata, got %v", m)
	}
}

func TestSQLiteCosineOrdering(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	// Identical vector must score ~1 and come first; orthogonal next; opposite last.
	if err := s.Upsert(ctx, vectorstore.Vector{ID: "same", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "ortho", Embedding: []float32{0, 1}}); err != nil {
		t.Fatal(err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "opp", Embedding: []float32{-1, 0}}); err != nil {
		t.Fatal(err)
	}

	results, err := s.Query(ctx, []float32{1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %v", results)
	}

	if results[0].ID != "same" || results[0].Score < 0.999 {
		t.Fatalf("expected identical first with score ~1, got %s %f", results[0].ID, results[0].Score)
	}

	if results[1].ID != "ortho" {
		t.Fatalf("expected ortho second, got %s", results[1].ID)
	}

	if results[2].ID != "opp" {
		t.Fatalf("expected opp last, got %s", results[2].ID)
	}

	if !(results[0].Score > results[1].Score && results[1].Score > results[2].Score) {
		t.Fatalf("scores not strictly descending: %v", results)
	}
}

func TestSQLiteUpsertUnserializableMetadata(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	err := s.Upsert(context.Background(), vectorstore.Vector{
		ID:        "v1",
		Embedding: []float32{1, 0},
		Metadata:  map[string]any{"f": func() {}},
	})
	if err == nil {
		t.Fatal("expected error for unserializable metadata")
	}
}

func TestSQLiteQueryCancelledContext(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := s.Query(cancelCtx, []float32{1, 0}, 1); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSQLiteScanRowsCancelledContext(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "v1", Embedding: []float32{1, 0}}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, embedding, metadata FROM vectors`)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = rows.Close() }()

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := s.scanRows(cancelCtx, rows, []float32{1, 0}, 1); err == nil {
		t.Fatal("expected context error from scanRows")
	}
}

func TestNewInvalidDSN(t *testing.T) {
	t.Parallel()

	s, err := New(vectorstore.Options{DSN: "bad;dsn"})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}

	if s != nil {
		t.Fatal("expected nil store for invalid DSN")
	}
}

// errBoom is a sentinel for driver-failure injection tests.
var errBoom = errors.New("boom")

// failConnector builds *sql.DB instances backed by a scripted driver so
// row-iteration failure branches stay covered without a live failure.
type failConnector struct {
	nextErr  error
	closeErr error
	oneCol   bool
}

func (c failConnector) Connect(context.Context) (driver.Conn, error) {
	return failConn(c), nil
}

func (c failConnector) Driver() driver.Driver { return failDriver{} }

type failDriver struct{}

func (failDriver) Open(string) (driver.Conn, error) { return failConn{}, nil }

type failConn struct {
	nextErr  error
	closeErr error
	oneCol   bool
}

func (c failConn) Prepare(string) (driver.Stmt, error) { return nil, errBoom }
func (c failConn) Close() error                        { return nil }
func (c failConn) Begin() (driver.Tx, error)           { return nil, errBoom }

func (c failConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &failRows{nextErr: c.nextErr, closeErr: c.closeErr, oneCol: c.oneCol}, nil
}

type failRows struct {
	nextErr  error
	closeErr error
	oneCol   bool
	done     bool
}

func (r *failRows) Columns() []string {
	if r.oneCol {
		return []string{"c"}
	}

	return []string{"id", "embedding", "metadata"}
}
func (r *failRows) Close() error { return r.closeErr }

func (r *failRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}

	r.done = true

	if r.nextErr != nil {
		return r.nextErr
	}

	// One well-typed row: id string, 4-byte float32 blob, NULL metadata.
	// Writes are len-guarded: one-column variants size dest to 1.
	if len(dest) > 0 {
		dest[0] = "x"
	}

	if len(dest) > 1 {
		dest[1] = []byte{0x00, 0x00, 0x80, 0x3F}
	}

	return nil
}

func newFailStore(nextErr, closeErr error) *Store {
	return &Store{db: sql.OpenDB(failConnector{nextErr: nextErr, closeErr: closeErr})}
}

func TestDelete_RowsErr(t *testing.T) {
	t.Parallel()

	s := newFailStore(errBoom, nil)
	defer func() { _ = s.Close() }()

	if err := s.Delete(context.Background(), "x"); !errors.Is(err, errBoom) {
		t.Fatalf("Delete err = %v, want boom", err)
	}
}

func TestDelete_RowsCloseErr(t *testing.T) {
	t.Parallel()

	s := newFailStore(nil, errBoom)
	defer func() { _ = s.Close() }()

	if err := s.Delete(context.Background(), "x"); !errors.Is(err, errBoom) {
		t.Fatalf("Delete err = %v, want boom", err)
	}
}

func TestQuery_ScanRowsErr(t *testing.T) {
	t.Parallel()

	s := newFailStore(errBoom, nil)
	defer func() { _ = s.Close() }()

	if _, err := s.Query(context.Background(), []float32{1}, 1); !errors.Is(err, errBoom) {
		t.Fatalf("Query err = %v, want boom", err)
	}
}

func TestQuery_AutoCloseErr(t *testing.T) {
	t.Parallel()

	s := newFailStore(nil, errBoom)
	defer func() { _ = s.Close() }()

	// database/sql auto-closes rows on EOF; the auto-close failure lands in
	// rows.Err(), so scanRows (not an explicit Close check) reports it.
	if _, err := s.Query(context.Background(), []float32{1}, 1); !errors.Is(err, errBoom) {
		t.Fatalf("Query err = %v, want boom", err)
	}
}

func TestDecodeRow_ScanColumnMismatch(t *testing.T) {
	t.Parallel()

	// One-column rows against a three-destination Scan: database/sql rejects
	// the arity mismatch deterministically.
	db := sql.OpenDB(failConnector{oneCol: true})
	s := &Store{db: db}
	defer func() { _ = s.Close() }()

	_, err := s.Query(context.Background(), []float32{1}, 1)
	if err == nil {
		t.Fatal("expected scan error for column count mismatch")
	}
}

func TestDecodeRow_ScanTypeMismatch(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	// INTEGER in a BLOB column: convertAssign rejects int64 -> []byte.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO vectors (id, embedding, metadata) VALUES ('bad', 42, NULL)`); err != nil {
		t.Fatalf("setup insert: %v", err)
	}

	if _, err := s.Query(ctx, []float32{1}, 1); err == nil {
		t.Fatal("expected scan error for integer embedding")
	}
}
