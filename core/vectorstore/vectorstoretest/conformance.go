// Package vectorstoretest provides the conformance kit third-party vectorstore adapters run to prove backend parity.
package vectorstoretest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
)

// Conformance verifies factory-built vector stores implement the
// vectorstore.VectorStore contract: Upsert/UpsertBatch round-trip through
// Query with cosine-similarity ranking, overwrite on re-upsert, Delete,
// empty-embedding and dimension-mismatch sentinels, invalid-metadata
// rejection, default topK behavior, and Close. Each subtest takes a fresh
// instance from factory and uses its own ID prefix so cases stay isolated
// even on shared live servers. Tests are deterministic and touch no network.
//
// Delete-missing tolerance: sqlite and pgvector report NotFound for a missing
// id while qdrant delete is idempotent and reports no error (see the
// VectorStore interface docs). The kit accepts either nil or ErrNotFound so
// both behaviors conform.
func Conformance(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	t.Run("UpsertQuery", func(t *testing.T) { conformanceUpsertQuery(t, factory) })
	t.Run("UpsertBatch", func(t *testing.T) { conformanceUpsertBatch(t, factory) })
	t.Run("Overwrite", func(t *testing.T) { conformanceOverwrite(t, factory) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("DeleteMissing", func(t *testing.T) { conformanceDeleteMissing(t, factory) })
	t.Run("EmptyEmbedding", func(t *testing.T) { conformanceEmptyEmbedding(t, factory) })
	t.Run("DimensionMismatch", func(t *testing.T) { conformanceDimensionMismatch(t, factory) })
	t.Run("InvalidMetadata", func(t *testing.T) { conformanceInvalidMetadata(t, factory) })
	t.Run("TopKDefault", func(t *testing.T) { conformanceTopKDefault(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

// mustUpsert fails the test on Upsert error.
func mustUpsert(t *testing.T, s vectorstore.VectorStore, vec vectorstore.Vector) {
	t.Helper()

	if err := s.Upsert(t.Context(), vec); err != nil {
		t.Fatalf("Upsert(%q) error = %v", vec.ID, err)
	}
}

// mustQuery fails the test on Query error.
func mustQuery(t *testing.T, s vectorstore.VectorStore, embedding []float32, topK int) []vectorstore.ScoreMatch {
	t.Helper()

	matches, err := s.Query(t.Context(), embedding, topK)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	return matches
}

func conformanceUpsertQuery(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)

	mustUpsert(t, s, vectorstore.Vector{ID: "kit-uq-one", Embedding: []float32{1, 0, 0}, Metadata: map[string]any{"name": "alpha"}})
	mustUpsert(t, s, vectorstore.Vector{ID: "kit-uq-two", Embedding: []float32{0, 1, 0}, Metadata: map[string]any{"name": "beta"}})

	matches := mustQuery(t, s, []float32{1, 0, 0}, 2)

	if len(matches) != 2 {
		t.Fatalf("Query() matches = %v, want 2", matches)
	}

	if matches[0].ID != "kit-uq-one" {
		t.Errorf("Query()[0].ID = %q, want kit-uq-one", matches[0].ID)
	}

	if matches[0].Score < 0.99 {
		t.Errorf("Query()[0].Score = %v, want ~1", matches[0].Score)
	}

	if matches[0].Metadata["name"] != "alpha" {
		t.Errorf("Query()[0].Metadata = %v, want name alpha", matches[0].Metadata)
	}
}

func conformanceUpsertBatch(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)

	vecs := []vectorstore.Vector{
		{ID: "kit-ub-one", Embedding: []float32{1, 0, 0}},
		{ID: "kit-ub-two", Embedding: []float32{0, 1, 0}},
	}
	if err := s.UpsertBatch(t.Context(), vecs); err != nil {
		t.Fatalf("UpsertBatch() error = %v", err)
	}

	matches := mustQuery(t, s, []float32{0, 1, 0}, 2)

	if len(matches) != 2 {
		t.Fatalf("Query() matches = %v, want 2", matches)
	}

	if matches[0].ID != "kit-ub-two" {
		t.Errorf("Query()[0].ID = %q, want kit-ub-two", matches[0].ID)
	}
}

func conformanceOverwrite(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)

	mustUpsert(t, s, vectorstore.Vector{ID: "kit-ow-one", Embedding: []float32{1, 0, 0}})
	mustUpsert(t, s, vectorstore.Vector{ID: "kit-ow-two", Embedding: []float32{0, 1, 0}})

	// Re-upsert kit-ow-two onto a distant axis with fresh metadata; the
	// replace must move it to the top for its new neighborhood and carry
	// the new metadata, proving overwrite rather than duplicate insert.
	mustUpsert(t, s, vectorstore.Vector{ID: "kit-ow-two", Embedding: []float32{0, 0, 1}, Metadata: map[string]any{"v": "two"}})

	matches := mustQuery(t, s, []float32{0, 0, 1}, 10)

	if len(matches) != 2 {
		t.Fatalf("Query() matches = %v, want 2 (replace, not duplicate)", matches)
	}

	if matches[0].ID != "kit-ow-two" {
		t.Errorf("Query()[0].ID = %q, want kit-ow-two after overwrite", matches[0].ID)
	}

	if matches[0].Metadata["v"] != "two" {
		t.Errorf("Query()[0].Metadata = %v, want v two after overwrite", matches[0].Metadata)
	}
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)
	ctx := t.Context()

	mustUpsert(t, s, vectorstore.Vector{ID: "kit-del-one", Embedding: []float32{1, 0, 0}})
	mustUpsert(t, s, vectorstore.Vector{ID: "kit-del-two", Embedding: []float32{0, 1, 0}})

	if err := s.Delete(ctx, "kit-del-one"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	matches := mustQuery(t, s, []float32{1, 0, 0}, 2)

	if len(matches) != 1 || matches[0].ID != "kit-del-two" {
		t.Errorf("Query() = %v, want only kit-del-two after delete", matches)
	}
}

func conformanceDeleteMissing(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	err := factory(t).Delete(t.Context(), "kit-no-such-vector")
	if err == nil {
		// Idempotent backends (qdrant) report no error; accepted per interface docs.
		return
	}

	var nfErr vectorstore.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("Delete(missing) err = %T %v, want *NotFoundError or nil", err, err)
	}

	if nfErr.ID != "kit-no-such-vector" {
		t.Errorf("NotFoundError.ID = %q, want kit-no-such-vector", nfErr.ID)
	}

	if !errors.Is(err, vectorstore.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
}

func conformanceEmptyEmbedding(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)
	ctx := t.Context()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "kit-ee-one"}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Upsert(nil embedding) err = %v, want ErrEmptyEmbedding", err)
	}

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "kit-ee-two", Embedding: []float32{}}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Upsert(empty embedding) err = %v, want ErrEmptyEmbedding", err)
	}

	if _, err := s.Query(ctx, nil, 5); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Query(nil embedding) err = %v, want ErrEmptyEmbedding", err)
	}

	if _, err := s.Query(ctx, []float32{}, 5); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Query(empty embedding) err = %v, want ErrEmptyEmbedding", err)
	}

	if err := s.UpsertBatch(ctx, []vectorstore.Vector{{ID: "kit-ee-three"}}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("UpsertBatch(empty embedding) err = %v, want ErrEmptyEmbedding", err)
	}
}

func conformanceDimensionMismatch(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)
	ctx := t.Context()

	mustUpsert(t, s, vectorstore.Vector{ID: "kit-dm-one", Embedding: []float32{1, 0, 0}})

	err := s.Upsert(ctx, vectorstore.Vector{ID: "kit-dm-two", Embedding: []float32{1, 0}})

	var mmErr vectorstore.DimensionMismatchError
	if !errors.As(err, &mmErr) {
		t.Fatalf("Upsert(wrong dim) err = %T %v, want *DimensionMismatchError", err, err)
	}

	if mmErr.Got != 2 || mmErr.Want != 3 {
		t.Errorf("DimensionMismatchError = got %d want %d, want got 2 want 3", mmErr.Got, mmErr.Want)
	}

	if !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Errorf("errors.Is(err, ErrDimensionMismatch) = false (err = %v)", err)
	}

	if _, err := s.Query(ctx, []float32{1, 0}, 5); !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Errorf("Query(wrong dim) err = %v, want ErrDimensionMismatch", err)
	}
}

func conformanceInvalidMetadata(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)

	bad := vectorstore.Vector{
		ID:        "kit-bad-meta",
		Embedding: []float32{1, 0, 0},
		Metadata:  map[string]any{"ch": make(chan int)},
	}

	if err := s.Upsert(t.Context(), bad); !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Errorf("Upsert(bad metadata) err = %v, want ErrInvalidMetadata", err)
	}

	if err := s.UpsertBatch(t.Context(), []vectorstore.Vector{bad}); !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Errorf("UpsertBatch(bad metadata) err = %v, want ErrInvalidMetadata", err)
	}
}

func conformanceTopKDefault(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)
	ctx := t.Context()

	mustUpsert(t, s, vectorstore.Vector{ID: "kit-tk-one", Embedding: []float32{1, 0, 0}})

	for _, topK := range []int{0, -1} {
		matches, err := s.Query(ctx, []float32{1, 0, 0}, topK)
		if err != nil {
			t.Fatalf("Query(topK %d) error = %v", topK, err)
		}

		if len(matches) != 1 || matches[0].ID != "kit-tk-one" {
			t.Errorf("Query(topK %d) = %v, want kit-tk-one", topK, matches)
		}
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) vectorstore.VectorStore) {
	t.Helper()

	s := factory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
