package rag

import (
	"context"
	"fmt"
	"sort"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// DefaultSearchIndex names the keyword index hybrid engines read and write.
const DefaultSearchIndex = "rag"

// DefaultRRFK is the Reciprocal Rank Fusion constant: a fused candidate at
// rank r in one list scores 1/(RRFK+r).
const DefaultRRFK = 60

// HybridOptions configures hybrid retrieval.
type HybridOptions struct {
	// TopK caps returned sources; <= 0 selects Options.TopK.
	TopK int
	// Filter keeps only sources whose metadata matches every pair, compared
	// as strings; nil disables filtering.
	Filter map[string]string
	// RRFK is the fusion constant; <= 0 selects DefaultRRFK.
	RRFK int
	// Reranker optionally rescores fused sources after filtering and before
	// the topK cut; nil skips reranking.
	Reranker Reranker
}

// NewHybrid builds an Engine that additionally indexes into search, enabling
// hybrid retrieval. It validates all three dependencies.
func NewHybrid(client ai.AI, store vectorstore.VectorStore, index search.Search, opts Options) (*Engine, error) {
	if index == nil {
		return nil, ErrNoSearch
	}

	e, err := New(client, store, opts)
	if err != nil {
		return nil, err
	}

	e.search = index

	return e, nil
}

// RetrieveHybrid fuses vector similarity with keyword search through
// Reciprocal Rank Fusion and returns the topK fused sources. A topK <= 0
// selects Options.TopK. It fails on engines built without a search backend.
func (e *Engine) RetrieveHybrid(ctx context.Context, query string, topK int, opts HybridOptions) (sources []Source, err error) {
	defer func() {
		e.observe(Event{Type: EventRetrieve, Query: query, Count: len(sources), Err: err})
	}()

	return e.retrieveHybrid(ctx, query, topK, opts)
}

// retrieveHybrid implements RetrieveHybrid.
func (e *Engine) retrieveHybrid(ctx context.Context, query string, topK int, opts HybridOptions) ([]Source, error) {
	if e.search == nil {
		return nil, ErrNoSearch
	}
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if topK <= 0 {
		topK = e.opts.TopK
	}
	if opts.TopK > 0 {
		topK = opts.TopK
	}

	k := opts.RRFK
	if k <= 0 {
		k = DefaultRRFK
	}

	pool := 2 * topK
	if pool < 2 {
		pool = 2
	}

	embs, err := e.client.Embed(ctx, e.opts.embedModel(), []string{query}, ai.EmbedOptions{})
	if err != nil {
		return nil, err
	}
	if len(embs) != 1 {
		return nil, EmbeddingCountError{Got: len(embs), Want: 1}
	}

	matches, err := e.store.Query(ctx, embs[0], pool)
	if err != nil {
		return nil, err
	}

	res, err := e.search.Search(ctx, query, search.QueryOptions{
		Limit:   pool,
		Filters: map[string]string{"index": e.searchIndex()},
	})
	if err != nil {
		return nil, err
	}

	sources := fuseRRF(matches, res.Hits, k)

	filtered := sources[:0]
	for _, s := range sources {
		if matchFilter(s.Metadata, opts.Filter) {
			filtered = append(filtered, s)
		}
	}

	if opts.Reranker != nil {
		reranked, err := opts.Reranker.Rerank(ctx, query, filtered)
		if err != nil {
			return nil, err
		}

		filtered = reranked
	}

	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	e.observe(Event{Type: EventRetrieve, Query: query, Count: len(filtered)})

	return filtered, nil
}

// AnswerHybrid retrieves hybrid context for query and generates a grounded
// answer.
func (e *Engine) AnswerHybrid(ctx context.Context, query string, topK int, opts HybridOptions) (Answer, error) {
	sources, err := e.RetrieveHybrid(ctx, query, topK, opts)
	if err != nil {
		return Answer{}, err
	}

	return e.generate(ctx, query, sources)
}

// fuseRRF merges vector matches and keyword hits by Reciprocal Rank Fusion,
// highest fused score first with ID tiebreak for determinism.
func fuseRRF(matches []vectorstore.ScoreMatch, hits []search.Hit, k int) []Source {
	type fused struct {
		content string
		meta    map[string]any
		score   float64
	}

	acc := map[string]*fused{}

	add := func(id string, rank int, meta map[string]any) {
		f, ok := acc[id]
		if !ok {
			content, _ := meta["content"].(string)

			// Copy the map so callers cannot alias store internals, and so
			// a second backend's metadata cannot overwrite the first.
			copied := make(map[string]any, len(meta))
			for k, v := range meta {
				copied[k] = v
			}

			f = &fused{content: content, meta: copied}
			acc[id] = f
		}

		f.score += 1.0 / float64(k+rank+1)
	}

	for rank, m := range matches {
		add(m.ID, rank, m.Metadata)
	}

	for rank, h := range hits {
		add(h.ID, rank, h.Metadata)
	}

	sources := make([]Source, 0, len(acc))
	for id, f := range acc {
		sources = append(sources, Source{
			ID:       id,
			Score:    float32(f.score),
			Content:  f.content,
			Metadata: f.meta,
		})
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Score == sources[j].Score {
			return sources[i].ID < sources[j].ID
		}

		return sources[i].Score > sources[j].Score
	})

	return sources
}

// matchFilter reports whether metadata satisfies every filter pair, compared
// as strings. A nil filter matches everything.
func matchFilter(meta map[string]any, filter map[string]string) bool {
	for k, want := range filter {
		got, ok := meta[k]
		if !ok {
			return false
		}

		if fmt.Sprintf("%v", got) != want {
			return false
		}
	}

	return true
}
