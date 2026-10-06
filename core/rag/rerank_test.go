package rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

func TestTermOverlapReranker(t *testing.T) {
	t.Parallel()

	sources := []Source{
		{ID: "a", Content: "unrelated weather report"},
		{ID: "b", Content: "the fox jumps over dogs"},
		{ID: "c", Content: "quick fox runs quick"},
	}

	out, err := TermOverlapReranker{}.Rerank(t.Context(), "quick fox", sources)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if len(out) != 3 {
		t.Fatalf("sources = %d, want 3", len(out))
	}

	if out[0].ID != "c" || out[1].ID != "b" || out[2].ID != "a" {
		t.Fatalf("order = %v, want c b a", idsOf(out))
	}
}

func TestTermOverlapReranker_empty(t *testing.T) {
	t.Parallel()

	out, err := TermOverlapReranker{}.Rerank(t.Context(), "q", nil)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if len(out) != 0 {
		t.Fatalf("sources = %d, want 0", len(out))
	}
}

func TestTermOverlapReranker_canceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	sources := []Source{{ID: "a"}}
	reranker := TermOverlapReranker{}

	if _, err := reranker.Rerank(ctx, "q", sources); err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func idsOf(sources []Source) []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.ID)
	}

	return out
}

// judgeStubAI answers GenerateStructured-style generations with a fixed score.
type judgeStubAI struct {
	fakeAI
	score float64
}

func (s *judgeStubAI) Generate(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	raw, _ := json.Marshal(map[string]any{"score": s.score})

	return ai.Generation{Content: string(raw)}, nil
}

func TestJudgeReranker(t *testing.T) {
	client := &judgeStubAI{score: 0.7}

	out, err := JudgeReranker{Client: client, Model: "m"}.Rerank(t.Context(), "q", []Source{
		{ID: "a", Content: "first"},
		{ID: "b", Content: "second"},
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("sources = %d, want 2", len(out))
	}

	for _, s := range out {
		if s.Score != 0.7 {
			t.Fatalf("score = %v, want 0.7", s.Score)
		}
	}
}

func TestJudgeReranker_clamps(t *testing.T) {
	client := &judgeStubAI{score: 9}

	out, err := JudgeReranker{Client: client, Model: "m"}.Rerank(t.Context(), "q", []Source{
		{ID: "a", Content: "x"},
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if out[0].Score != 1 {
		t.Fatalf("score = %v, want clamped 1", out[0].Score)
	}
}

func TestJudgeReranker_nilClient(t *testing.T) {
	reranker := JudgeReranker{}

	if _, err := reranker.Rerank(t.Context(), "q", []Source{{ID: "a"}}); err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
}

func TestRetrieveHybrid_reranks(t *testing.T) {
	f := &fakeAI{}
	s := &memStore{}
	idx := &hybridSearch{}
	e := newHybridEngine(t, f, s, idx, Options{ChunkSize: 64})

	docs := []Document{
		{ID: "d1", Content: "fox fox fox"},
		{ID: "d2", Content: "the quick brown fox jumps"},
	}

	if err := e.Ingest(t.Context(), docs); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	sources, err := e.RetrieveHybrid(t.Context(), "quick fox", 5, HybridOptions{Reranker: TermOverlapReranker{}})
	if err != nil {
		t.Fatalf("RetrieveHybrid() error = %v", err)
	}

	if len(sources) == 0 {
		t.Fatal("no sources returned")
	}

	if !strings.Contains(sources[0].Content, "quick brown fox") {
		t.Fatalf("top = %q, want the quick-brown-fox chunk", sources[0].Content)
	}
}
