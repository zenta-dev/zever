package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// hybridSearch is an in-memory keyword index: Search matches documents whose
// content contains the query as a substring.
type hybridSearch struct {
	docs []search.Document
}

func (s *hybridSearch) Index(_ context.Context, doc search.Document) error {
	s.docs = append(s.docs, doc)
	return nil
}

func (s *hybridSearch) IndexBatch(_ context.Context, docs []search.Document) error {
	s.docs = append(s.docs, docs...)
	return nil
}

func (s *hybridSearch) Delete(context.Context, string) error { return nil }

func (s *hybridSearch) Search(_ context.Context, query string, opts search.QueryOptions) (search.Result, error) {
	var hits []search.Hit

	for i, d := range s.docs {
		if opts.Filters["index"] != "" && d.Index != opts.Filters["index"] {
			continue
		}

		if !strings.Contains(strings.ToLower(d.Content), strings.ToLower(query)) {
			continue
		}

		hits = append(hits, search.Hit{
			ID:       d.ID,
			Score:    float64(len(s.docs) - i),
			Metadata: d.Metadata,
		})
	}

	if opts.Limit > 0 && len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}

	return search.Result{Hits: hits, Total: int64(len(hits))}, nil
}

func (s *hybridSearch) Close() error { return nil }

func newHybridEngine(t *testing.T, f *fakeAI, s *memStore, idx *hybridSearch, opts Options) *Engine {
	t.Helper()

	e, err := NewHybrid(f, s, idx, opts)
	if err != nil {
		t.Fatalf("NewHybrid() error = %v", err)
	}

	return e
}

func TestNewHybrid_validation(t *testing.T) {
	t.Parallel()

	if _, err := NewHybrid(nil, &memStore{}, &hybridSearch{}, Options{}); err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	if _, err := NewHybrid(&fakeAI{}, nil, &hybridSearch{}, Options{}); err == nil {
		t.Fatal("expected error for nil store, got nil")
	}

	if _, err := NewHybrid(&fakeAI{}, &memStore{}, nil, Options{}); err == nil {
		t.Fatal("expected error for nil search, got nil")
	}
}

func TestRetrieveHybrid_fusesBothBranches(t *testing.T) {
	f := &fakeAI{}
	s := &memStore{}
	idx := &hybridSearch{}
	e := newHybridEngine(t, f, s, idx, Options{ChunkSize: 64})

	docs := []Document{
		{ID: "d1", Content: "the quick brown fox jumps over the lazy dog"},
		{ID: "d2", Content: "totally unrelated weather report content here"},
	}

	if err := e.Ingest(t.Context(), docs); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	if len(s.vecs) == 0 {
		t.Fatal("no vectors stored")
	}

	if len(idx.docs) == 0 {
		t.Fatal("no keyword docs indexed")
	}

	sources, err := e.RetrieveHybrid(t.Context(), "fox", 5, HybridOptions{})
	if err != nil {
		t.Fatalf("RetrieveHybrid() error = %v", err)
	}

	if len(sources) == 0 {
		t.Fatal("no sources returned")
	}

	if !strings.Contains(sources[0].Content, "fox") {
		t.Fatalf("top source = %q, want the fox chunk first", sources[0].Content)
	}
}

func TestRetrieveHybrid_filter(t *testing.T) {
	f := &fakeAI{}
	s := &memStore{}
	idx := &hybridSearch{}
	e := newHybridEngine(t, f, s, idx, Options{ChunkSize: 64})

	docs := []Document{
		{ID: "d1", Content: "shared fox content", Metadata: map[string]any{"lang": "en"}},
		{ID: "d2", Content: "shared fox content", Metadata: map[string]any{"lang": "fr"}},
	}

	if err := e.Ingest(t.Context(), docs); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	sources, err := e.RetrieveHybrid(t.Context(), "fox", 5, HybridOptions{Filter: map[string]string{"lang": "fr"}})
	if err != nil {
		t.Fatalf("RetrieveHybrid() error = %v", err)
	}

	for _, src := range sources {
		if src.Metadata["lang"] != "fr" {
			t.Fatalf("unfiltered source: %+v", src)
		}
	}

	if len(sources) == 0 {
		t.Fatal("filter removed everything, want the fr chunk")
	}
}

func TestRetrieveHybrid_noSearch(t *testing.T) {
	e := newTestEngine(t, &fakeAI{}, &memStore{}, Options{})

	if _, err := e.RetrieveHybrid(t.Context(), "q", 3, HybridOptions{}); err == nil {
		t.Fatal("expected error without search backend, got nil")
	}
}

func TestAnswerHybrid(t *testing.T) {
	f := &fakeAI{gen: ai.Generation{Content: "hybrid answer"}}
	s := &memStore{}
	idx := &hybridSearch{}
	e := newHybridEngine(t, f, s, idx, Options{ChunkSize: 64, Model: "m"})

	if err := e.Ingest(t.Context(), []Document{{ID: "d", Content: "fox facts"}}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	ans, err := e.AnswerHybrid(t.Context(), "fox?", 3, HybridOptions{})
	if err != nil {
		t.Fatalf("AnswerHybrid() error = %v", err)
	}

	if ans.Text != "hybrid answer" {
		t.Fatalf("answer text = %q", ans.Text)
	}

	if len(ans.Sources) == 0 {
		t.Fatal("no sources attached")
	}
}

func TestFuseRRF(t *testing.T) {
	t.Parallel()

	matches := []vectorstore.ScoreMatch{
		{ID: "a", Score: 0.9, Metadata: map[string]any{"content": "A"}},
		{ID: "b", Score: 0.8, Metadata: map[string]any{"content": "B"}},
	}
	hits := []search.Hit{
		{ID: "b", Score: 10, Metadata: map[string]any{"content": "B"}},
		{ID: "c", Score: 9, Metadata: map[string]any{"content": "C"}},
	}

	sources := fuseRRF(matches, hits, 60)

	if len(sources) != 3 {
		t.Fatalf("sources = %d, want 3", len(sources))
	}

	// b appears in both lists: 1/61 + 1/61 beats a and c at 1/61 and 1/62.
	if sources[0].ID != "b" {
		t.Fatalf("top = %q, want b (present in both lists)", sources[0].ID)
	}
}
