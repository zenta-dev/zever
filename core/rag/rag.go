package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// defaultSystemPrompt grounds generation in retrieved context.
const defaultSystemPrompt = "Answer the question using only the provided context. " +
	"Cite sources as [n]. If the context is insufficient, say so."

// Engine is the built-in retrieval-augmented generation engine.
type Engine struct {
	client ai.AI
	store  vectorstore.VectorStore
	search search.Search
	opts   Options
}

// New builds an Engine over client and store. It validates both dependencies
// and resolves option defaults.
func New(client ai.AI, store vectorstore.VectorStore, opts Options) (*Engine, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if store == nil {
		return nil, ErrNilStore
	}
	if opts.ChunkSize < 0 {
		return nil, InvalidOptionsError{Reason: "chunk_size must not be negative"}
	}

	return &Engine{client: client, store: store, opts: opts.withDefaults()}, nil
}

// Ingest chunks each document, embeds every chunk in one batch, and upserts the
// resulting vectors. Documents with empty content are skipped. The reserved
// metadata keys document_id, chunk_index and content are set by the engine;
// caller metadata with those keys is ignored. Engines built by NewHybrid
// additionally index every chunk into the keyword backend.
func (e *Engine) Ingest(ctx context.Context, docs []Document) error {
	err := e.ingest(ctx, docs)
	e.observe(Event{Type: EventIngest, Count: len(docs), Err: err})

	return err
}

// ingest implements Ingest.
func (e *Engine) ingest(ctx context.Context, docs []Document) error {
	type item struct {
		id     string
		meta   map[string]any
		textID int
	}

	var (
		texts []string
		items []item
	)

	for _, d := range docs {
		if err := d.Validate(); err != nil {
			return err
		}
		if d.Content == "" {
			continue
		}

		for i, c := range chunkText(d.Content, e.opts.ChunkSize, e.opts.ChunkOverlap) {
			meta := map[string]any{
				"document_id": d.ID,
				"chunk_index": i,
				"content":     c,
			}
			for k, v := range d.Metadata {
				if _, exists := meta[k]; !exists {
					meta[k] = v
				}
			}

			items = append(items, item{id: chunkID(d.ID, i), meta: meta, textID: len(texts)})
			texts = append(texts, c)
		}
	}

	if len(texts) == 0 {
		return nil
	}

	embs, err := e.client.Embed(ctx, e.opts.embedModel(), texts, ai.EmbedOptions{})
	if err != nil {
		return err
	}
	if len(embs) != len(texts) {
		return EmbeddingCountError{Got: len(embs), Want: len(texts)}
	}

	vecs := make([]vectorstore.Vector, len(items))
	for i, it := range items {
		vecs[i] = vectorstore.Vector{
			ID:        it.id,
			Embedding: embs[it.textID],
			Metadata:  it.meta,
		}
	}

	if err := e.store.UpsertBatch(ctx, vecs); err != nil {
		return err
	}

	if e.search == nil {
		return nil
	}

	sdocs := make([]search.Document, len(items))
	for i, it := range items {
		sdocs[i] = search.Document{
			ID:       it.id,
			Index:    e.searchIndex(),
			Content:  texts[it.textID],
			Metadata: it.meta,
		}
	}

	if err := e.search.IndexBatch(ctx, sdocs); err != nil {
		return fmt.Errorf("rag: vectors stored but keyword index failed: %w", err)
	}

	return nil
}

// Retrieve embeds query and returns the topK most similar stored chunks. A
// topK <= 0 selects Options.TopK.
func (e *Engine) Retrieve(ctx context.Context, query string, topK int) ([]Source, error) {
	sources, err := e.retrieve(ctx, query, topK)
	e.observe(Event{Type: EventRetrieve, Query: query, Count: len(sources), Err: err})

	return sources, err
}

// retrieve implements Retrieve.
func (e *Engine) retrieve(ctx context.Context, query string, topK int) ([]Source, error) {
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if topK <= 0 {
		topK = e.opts.TopK
	}

	embs, err := e.client.Embed(ctx, e.opts.embedModel(), []string{query}, ai.EmbedOptions{})
	if err != nil {
		return nil, err
	}
	if len(embs) != 1 {
		return nil, EmbeddingCountError{Got: len(embs), Want: 1}
	}

	matches, err := e.store.Query(ctx, embs[0], topK)
	if err != nil {
		return nil, err
	}

	sources := make([]Source, 0, len(matches))
	for _, m := range matches {
		content, _ := m.Metadata["content"].(string)
		sources = append(sources, Source{
			ID:       m.ID,
			Score:    m.Score,
			Content:  content,
			Metadata: m.Metadata,
		})
	}

	return sources, nil
}

// Answer retrieves context for query and generates a grounded answer.
func (e *Engine) Answer(ctx context.Context, query string, topK int) (Answer, error) {
	sources, err := e.Retrieve(ctx, query, topK)
	if err != nil {
		return Answer{}, err
	}

	return e.generate(ctx, query, sources)
}

// generate builds the grounded prompt from sources and generates an answer.
func (e *Engine) generate(ctx context.Context, query string, sources []Source) (Answer, error) {
	gen, err := func() (ai.Generation, error) {
		var b strings.Builder
		for i, s := range sources {
			fmt.Fprintf(&b, "[%d] %s\n", i+1, s.Content)
		}

		system := e.opts.SystemPrompt
		if system == "" {
			system = defaultSystemPrompt
		}

		msgs := []ai.Message{
			{Role: ai.RoleSystem, Content: system},
			{Role: ai.RoleUser, Content: "Context:\n" + b.String() + "\nQuestion: " + query},
		}

		return e.client.Generate(ctx, e.opts.Model, msgs, ai.GenerateOptions{})
	}()

	e.observe(Event{Type: EventGenerate, Query: query, Err: err})

	if err != nil {
		return Answer{}, err
	}

	return Answer{Text: gen.Content, Sources: sources, Usage: gen.Usage}, nil
}

// observe delivers ev to the configured observer, if any.
func (e *Engine) observe(ev Event) {
	if e.opts.Observe != nil {
		e.opts.Observe(ev)
	}
}

// searchIndex returns the keyword index name, defaulting when unset.
func (e *Engine) searchIndex() string {
	if e.opts.SearchIndex != "" {
		return e.opts.SearchIndex
	}

	return DefaultSearchIndex
}

// chunkID builds a stable chunk identifier from a document ID and chunk index.
func chunkID(docID string, index int) string {
	return fmt.Sprintf("%s#%d", docID, index)
}
