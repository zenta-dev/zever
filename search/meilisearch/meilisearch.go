package meilisearch

import (
	"context"
	"fmt"

	"github.com/meilisearch/meilisearch-go"

	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/internal/lrucache"
	"github.com/zenta-dev/zever/search"
)

const maxIDIndexes = 10000

var (
	idCodec       = codec.JSONCodec[string]{}
	scoreCodec    = codec.JSONCodec[float64]{}
	metadataCodec = codec.JSONCodec[map[string]any]{}
)

// cloneIndexes returns an independent copy of indexes, so neither the
// caller nor the idIndexes cache can mutate the other's backing array
// through an aliased slice. lrucache.Cache stores and returns values as-is
// (no cloning), so this defensive copy, previously done inside
// idIndexTracker's add/get, now has to happen at each call site that reads
// from or writes to the cache.
func cloneIndexes(indexes []string) []string {
	return append([]string(nil), indexes...)
}

func resolveIndex(filters map[string]string) (string, error) {
	if idx, ok := filters["index"]; ok && idx != "" {
		return idx, nil
	}

	return "", ErrIndexRequired
}

type meilisearchClient struct {
	client    meilisearch.ServiceManager
	idIndexes *lrucache.Cache[string, []string]
}

// New creates a Meilisearch-backed search.Search from options.
func New(o search.Options) (search.Search, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("meilisearch: %w", err)
	}

	if o.Host == "" {
		return nil, ErrMissingHost
	}

	var client meilisearch.ServiceManager
	if o.APIKey != "" {
		client = meilisearch.New(o.Host, meilisearch.WithAPIKey(o.APIKey))
	} else {
		client = meilisearch.New(o.Host)
	}

	return &meilisearchClient{client: client, idIndexes: lrucache.New[string, []string](maxIDIndexes)}, nil
}

func recordIndex(indexes []string, idx string) []string {
	for _, i := range indexes {
		if i == idx {
			return indexes
		}
	}

	return append(indexes, idx)
}

func (c *meilisearchClient) trackIndex(id, idx string) {
	cur, ok := c.idIndexes.Get(id)
	if ok {
		cur = cloneIndexes(cur)
	}

	c.idIndexes.Put(id, cloneIndexes(recordIndex(cur, idx)))
}

// Index adds or replaces doc in its index.
func (c *meilisearchClient) Index(ctx context.Context, doc search.Document) error {
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("meilisearch: index: %w", err)
	}

	// Uses AddDocumentsWithContext (available since meilisearch-go v0.28):
	// context-aware variant avoids the noctx lint hit from AddDocuments.
	metadata := make(map[string]any, len(doc.Metadata))
	for k, v := range doc.Metadata {
		metadata[k] = v
	}

	idx := c.client.Index(doc.Index)

	_, err := idx.AddDocumentsWithContext(ctx, []map[string]any{
		{
			"id":       doc.ID,
			"content":  doc.Content,
			"metadata": metadata,
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("meilisearch: index: %w", err)
	}

	c.trackIndex(doc.ID, doc.Index)

	return nil
}

// IndexBatch adds or replaces all of docs using meilisearch's native bulk
// AddDocuments API. Documents are grouped by Index, since meilisearch's batch
// endpoint targets one index per call, so this issues one HTTP call per
// distinct index among docs rather than one call per document. An empty docs
// is a no-op.
func (c *meilisearchClient) IndexBatch(ctx context.Context, docs []search.Document) error {
	if len(docs) == 0 {
		return nil
	}

	for i, doc := range docs {
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("meilisearch: index batch: index %d: %w", i, err)
		}
	}

	byIndex := make(map[string][]map[string]any)
	order := make([]string, 0)

	for _, doc := range docs {
		metadata := make(map[string]any, len(doc.Metadata))
		for k, v := range doc.Metadata {
			metadata[k] = v
		}

		if _, ok := byIndex[doc.Index]; !ok {
			order = append(order, doc.Index)
		}

		byIndex[doc.Index] = append(byIndex[doc.Index], map[string]any{
			"id":       doc.ID,
			"content":  doc.Content,
			"metadata": metadata,
		})
	}

	for _, idxName := range order {
		idx := c.client.Index(idxName)

		if _, err := idx.AddDocumentsWithContext(ctx, byIndex[idxName], nil); err != nil {
			return fmt.Errorf("meilisearch: index batch: %w", err)
		}
	}

	for _, doc := range docs {
		c.trackIndex(doc.ID, doc.Index)
	}

	return nil
}

// Delete removes the document with id from every tracked index.
func (c *meilisearchClient) Delete(ctx context.Context, id string) error {
	indexes, ok := c.idIndexes.Get(id)
	if ok {
		indexes = cloneIndexes(indexes)
	}

	if len(indexes) == 0 {
		// notFound is typed as error first: go vet's printf check rejects
		// %w with *NotFoundError directly (value-receiver Error method);
		// same pattern as payment/stub. errors.As(*NotFoundError) and
		// errors.Is(ErrNotFound) both still hold.
		notFound := error(&search.NotFoundError{ID: id})
		return fmt.Errorf(
			"%w: index for id %q not tracked; idIndexes is in-memory, populated only by Index and Search calls, "+
				"lost on restart and unsafe with multiple instances; hint: run a Search for the id in its index "+
				"(hits are re-tracked) or re-Index before Delete",
			notFound, id,
		)
	}

	for _, name := range indexes {
		_, err := c.client.Index(name).DeleteDocumentWithContext(ctx, id, nil)
		if err != nil {
			return fmt.Errorf("meilisearch: delete: %w", err)
		}
	}

	c.idIndexes.Delete(id)

	return nil
}

// Search runs query against the explicit index filter and returns ranked hits.
// An empty query short-circuits to an empty result without any HTTP call.
func (c *meilisearchClient) Search(ctx context.Context, query string, opts search.QueryOptions) (search.Result, error) {
	// Uses SearchWithContext (available since meilisearch-go v0.28):
	// context-aware variant avoids the noctx lint hit from Search.
	if opts.Limit <= 0 {
		opts.Limit = search.DefaultLimit
	}

	if opts.Offset < 0 {
		opts.Offset = 0
	}

	if query == "" {
		return search.Result{}, nil
	}

	idxName, err := resolveIndex(opts.Filters)
	if err != nil {
		return search.Result{}, err
	}

	idx := c.client.Index(idxName)

	searchReq := &meilisearch.SearchRequest{
		Query:            query,
		Limit:            int64(opts.Limit),
		Offset:           int64(opts.Offset),
		ShowRankingScore: true,
	}

	result, err := idx.SearchWithContext(ctx, query, searchReq)
	if err != nil {
		return search.Result{}, fmt.Errorf("meilisearch: search: %w", err)
	}

	hits := toHits(result.Hits)

	for _, h := range hits {
		if h.ID != "" {
			c.trackIndex(h.ID, idxName)
		}
	}

	return search.Result{
		Hits:  hits,
		Total: result.TotalHits,
	}, nil
}

func toHits(raw meilisearch.Hits) []search.Hit {
	var hits []search.Hit //nolint:prealloc // verbatim port of reference toHits; append preserves hit order.

	for _, r := range raw {
		hit := search.Hit{}

		if idRaw, ok := r["id"]; ok {
			if id, err := idCodec.Decode(idRaw); err == nil {
				hit.ID = id
			}
		}

		if scoreRaw, ok := r["_rankingScore"]; ok {
			if score, err := scoreCodec.Decode(scoreRaw); err == nil {
				hit.Score = score
			}
		}

		if metaRaw, ok := r["metadata"]; ok {
			if meta, err := metadataCodec.Decode(metaRaw); err == nil {
				hit.Metadata = meta
			}
		}

		hits = append(hits, hit)
	}

	return hits
}

// Close releases backend resources.
func (c *meilisearchClient) Close() error {
	return nil
}
