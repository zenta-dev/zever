package qdrant

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/qdrant/go-client/qdrant"

	"github.com/zenta-dev/zever/core/vectorstore"
)

func TestEdgeDeleteNonUUIDID(t *testing.T) {
	t.Parallel()

	f := newStub()
	s := newStore(f, 0)
	ctx := t.Context()

	key := pointID("not-a-uuid").GetUuid()
	f.points[key] = stored{vec: []float32{1}, payload: map[string]*qdrant.Value{}}

	if err := s.Delete(ctx, "not-a-uuid"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	f.mu.Lock()
	_, ok := f.points[key]
	f.mu.Unlock()

	if ok {
		t.Fatal("hashed point not removed")
	}
}

func TestEdgeDeleteEmptyID(t *testing.T) {
	t.Parallel()

	f := newStub()
	s := newStore(f, 0)
	ctx := t.Context()

	key := pointID("").GetUuid()
	if key == "" {
		t.Fatal("pointID(\"\") returned empty id")
	}

	f.points[key] = stored{vec: []float32{1}, payload: map[string]*qdrant.Value{}}

	if err := s.Delete(ctx, ""); err != nil {
		t.Fatalf("Delete(\"\") err = %v", err)
	}

	f.mu.Lock()
	_, ok := f.points[key]
	f.mu.Unlock()

	if ok {
		t.Fatal("empty-id point not removed")
	}
}

func TestEdgeDeleteErrorPropagation(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("edge delete boom")

	f := newStub()
	f.deleteErr = sentinel
	s := newStore(f, 0)

	err := s.Delete(t.Context(), "x")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Delete err = %v, want errors.Is sentinel", err)
	}

	if !strings.Contains(err.Error(), "qdrant: delete") {
		t.Fatalf("Delete err = %v, want qdrant: delete prefix", err)
	}
}

func TestEdgeUpsertBatchInvalidVectorIndex(t *testing.T) {
	t.Parallel()

	s := newStore(newStub(), 0)

	err := s.UpsertBatch(t.Context(), []vectorstore.Vector{
		{ID: "ok", Embedding: []float32{1}},
		{ID: "bad", Embedding: []float32{1}, Metadata: map[string]any{"fn": func() {}}},
	})
	if !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("UpsertBatch err = %v, want ErrInvalidVector", err)
	}

	if !strings.Contains(err.Error(), "index 1") {
		t.Fatalf("UpsertBatch err = %v, want index 1 annotation", err)
	}
}

func TestEdgeUpsertBatchSingleVector(t *testing.T) {
	t.Parallel()

	f := newStub()
	s := newStore(f, 0)

	if err := s.UpsertBatch(t.Context(), []vectorstore.Vector{
		{ID: "solo", Embedding: []float32{1, 0}},
	}); err != nil {
		t.Fatalf("UpsertBatch single err = %v", err)
	}

	f.mu.Lock()
	n := len(f.points)
	f.mu.Unlock()

	if n != 1 {
		t.Fatalf("points = %d, want 1", n)
	}
}

func TestEdgeQueryTopKBoundary(t *testing.T) {
	t.Parallel()

	f := newStub()
	s := newStore(f, 0)
	ctx := t.Context()

	for _, id := range []string{"a", "b", "c"} {
		if err := s.Upsert(ctx, vectorstore.Vector{ID: id, Embedding: []float32{1}}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}

	got, err := s.Query(ctx, []float32{1}, 1)
	if err != nil {
		t.Fatalf("Query topK=1 err = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("Query topK=1 = %d results, want 1", len(got))
	}

	got, err = s.Query(ctx, []float32{1}, -5)
	if err != nil {
		t.Fatalf("Query topK=-5 err = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Query topK=-5 = %d results, want default cap 3 (all stored)", len(got))
	}

	got, err = s.Query(ctx, []float32{1}, 100)
	if err != nil {
		t.Fatalf("Query topK=100 err = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Query topK=100 = %d results, want 3", len(got))
	}
}

func TestEdgeQueryEmptyStore(t *testing.T) {
	t.Parallel()

	s := newStore(newStub(), 0)

	got, err := s.Query(t.Context(), []float32{1}, 10)
	if err != nil {
		t.Fatalf("Query empty err = %v", err)
	}

	if got == nil {
		t.Fatal("Query empty = nil slice, want non-nil empty")
	}

	if len(got) != 0 {
		t.Fatalf("Query empty = %d results, want 0", len(got))
	}
}

func TestEdgeQueryDimensionExactMatch(t *testing.T) {
	t.Parallel()

	s := newStore(newStub(), 3)

	if _, err := s.Query(t.Context(), []float32{1, 2, 3}, 1); err != nil {
		t.Fatalf("Query exact-dim err = %v, want nil", err)
	}

	var dm vectorstore.DimensionMismatchError

	if _, err := s.Query(t.Context(), []float32{1, 2, 3, 4}, 1); !errors.As(err, &dm) {
		t.Fatalf("Query over-dim err = %v, want DimensionMismatchError", err)
	}
}

func TestEdgeExtractValueDeepNested(t *testing.T) {
	t.Parallel()

	in := &qdrant.Value{Kind: &qdrant.Value_ListValue{
		ListValue: &qdrant.ListValue{Values: []*qdrant.Value{
			{Kind: &qdrant.Value_StructValue{StructValue: &qdrant.Struct{Fields: map[string]*qdrant.Value{
				"inner": {Kind: &qdrant.Value_ListValue{ListValue: &qdrant.ListValue{Values: []*qdrant.Value{
					{Kind: &qdrant.Value_DoubleValue{DoubleValue: 2.5}},
				}}}},
			}}}},
		}},
	}}

	got := extractValue(in)

	list, ok := got.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("extractValue = %#v, want one-element list", got)
	}

	strct, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("extractValue[0] = %#v, want map", list[0])
	}

	inner, ok := strct["inner"].([]any)
	if !ok || len(inner) != 1 || inner[0] != 2.5 {
		t.Fatalf("extractValue deep inner = %#v, want [2.5]", strct["inner"])
	}
}

func TestEdgePointIDEdgeInputs(t *testing.T) {
	t.Parallel()

	empty := pointID("").GetUuid()
	if empty == "" {
		t.Fatal("pointID(\"\") = empty")
	}

	if !uuidPattern.MatchString(empty) {
		t.Fatalf("pointID(\"\") = %q, want valid UUID form", empty)
	}

	upper := "123E4567-E89B-12D3-A456-426614174000"
	if got := pointID(upper).GetUuid(); got != upper {
		t.Fatalf("pointID uppercase = %q, want passthrough %q", got, upper)
	}

	long := strings.Repeat("x", 10_000)
	a, b := pointID(long).GetUuid(), pointID(long).GetUuid()
	if a != b {
		t.Fatalf("pointID long not deterministic: %q vs %q", a, b)
	}

	if !uuidPattern.MatchString(a) {
		t.Fatalf("pointID long = %q, want valid UUID form", a)
	}
}

func TestEdgeEnsureCollectionRetryAfterFailure(t *testing.T) {
	t.Parallel()

	f := newStub()
	f.createErr = errors.New("edge create boom")
	s := newStore(f, 0)
	ctx := t.Context()

	if err := s.ensureCollection(ctx, 2); err == nil {
		t.Fatal("first ensure expected error")
	}

	if s.created {
		t.Fatal("failed ensure must leave created false")
	}

	f.createErr = nil

	if err := s.ensureCollection(ctx, 2); err != nil {
		t.Fatalf("retry ensure err = %v", err)
	}

	if !s.created {
		t.Fatal("retry ensure must set created")
	}

	if f.creates != 2 {
		t.Fatalf("CreateCollection calls = %d, want 2 (retry after failure)", f.creates)
	}
}

func TestEdgeConcurrentUpsertDeleteQuery(t *testing.T) {
	t.Parallel()

	f := newStub()
	s := newStore(f, 0)
	ctx := t.Context()

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			id := fmt.Sprintf("edge-%d", i)
			_, _ = s.Query(ctx, []float32{float32(i), 1}, 2)
			_ = s.Upsert(ctx, vectorstore.Vector{ID: id, Embedding: []float32{float32(i), 1}})
			_ = s.Delete(ctx, id)
		}()
	}

	wg.Wait()
}
