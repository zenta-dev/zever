package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/core/agent"
	"github.com/zenta-dev/zever/core/ai"
)

// Reranker rescores fused sources for query; higher wins. Rerankers run
// after fusion and filtering, before the topK cut.
type Reranker interface {
	// Rerank returns sources ordered best-first. It may reorder, drop, or
	// rescore entries but must not invent new IDs.
	Rerank(ctx context.Context, query string, sources []Source) ([]Source, error)
}

// TermOverlapReranker scores sources by distinct query-term overlap,
// case-insensitive on whitespace-separated tokens. It is deterministic and
// dependency-free, suitable as the default reranker and in tests.
type TermOverlapReranker struct{}

// Rerank orders sources by descending term overlap, preserving input order
// on ties.
func (TermOverlapReranker) Rerank(ctx context.Context, query string, sources []Source) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	terms := tokens(query)

	scored := make([]Source, len(sources))
	copy(scored, sources)

	sort.SliceStable(scored, func(i, j int) bool {
		return overlap(scored[i].Content, terms) > overlap(scored[j].Content, terms)
	})

	for i := range scored {
		scored[i].Score = float32(overlap(scored[i].Content, terms))
	}

	return scored, nil
}

// tokens lowercases s and splits it on whitespace, dropping empties.
func tokens(s string) []string {
	var out []string

	for _, tok := range strings.Fields(strings.ToLower(s)) {
		if tok != "" {
			out = append(out, tok)
		}
	}

	return out
}

// overlap counts distinct query terms appearing as substrings of content.
func overlap(content string, terms []string) int {
	lowered := strings.ToLower(content)

	seen := map[string]bool{}
	n := 0

	for _, term := range terms {
		if !seen[term] && strings.Contains(lowered, term) {
			seen[term] = true
			n++
		}
	}

	return n
}

// JudgeReranker asks an LLM to score each source 0..1 and orders best-first.
// Scores come back through agent.GenerateStructured against a fixed schema
// and are clamped; judge failures fail closed.
type JudgeReranker struct {
	// Client is the LLM backend. Required.
	Client ai.AI
	// Model selects the judging model.
	Model string
}

// judgeSchema constrains the judge to one number.
var judgeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"score": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
	},
	"required": []string{"score"},
}

// Rerank scores every source with the judge and orders best-first, keeping
// input order on ties.
func (j JudgeReranker) Rerank(ctx context.Context, query string, sources []Source) ([]Source, error) {
	if j.Client == nil {
		return nil, fmt.Errorf("rag: judge reranker missing client: %w", ErrInvalidOptions)
	}

	scores := make([]float64, len(sources))

	for i, s := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		score, err := j.scoreOne(ctx, query, s)
		if err != nil {
			return nil, err
		}

		scores[i] = score
	}

	scored := make([]Source, len(sources))
	copy(scored, sources)

	type ranked struct {
		src   Source
		score float64
	}

	items := make([]ranked, len(scored))
	for i := range scored {
		items[i] = ranked{src: scored[i], score: scores[i]}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	for i := range items {
		scored[i] = items[i].src
		scored[i].Score = float32(items[i].score)
	}

	return scored, nil
}

// scoreOne judges a single source.
func (j JudgeReranker) scoreOne(ctx context.Context, query string, s Source) (float64, error) {
	var out struct {
		Score float64 `json:"score"`
	}

	msgs := []ai.Message{{
		Role: ai.RoleUser,
		Content: "Rate how well the passage answers the question, 0 to 1.\n\nQuestion:\n" +
			query + "\n\nPassage:\n" + s.Content,
	}}

	if _, err := agent.GenerateStructured(ctx, j.Client, j.Model, msgs, judgeSchema, &out); err != nil {
		return 0, fmt.Errorf("rag: judge rerank failed: %w", err)
	}

	score := out.Score
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score, nil
}
