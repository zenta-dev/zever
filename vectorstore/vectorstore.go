package vectorstore

import (
	"context"
	"fmt"
	"sync"
)

// VectorStore defines the vector-similarity contract for vectorstore backends.
// Implementations return the zero value on error.
// Similarity is cosine similarity only; Score is higher for more similar vectors.
type VectorStore interface {
	// Upsert inserts or replaces vec. It returns ErrEmptyEmbedding for an empty embedding.
	Upsert(ctx context.Context, vec Vector) error
	// UpsertBatch inserts or replaces all of vecs. It returns ErrEmptyEmbedding for any empty embedding.
	UpsertBatch(ctx context.Context, vecs []Vector) error
	// Delete removes the vector with id.
	// The sqlite and pgvector backends report NotFound for a missing id,
	// while the qdrant backend delete is idempotent and reports no error.
	Delete(ctx context.Context, id string) error
	// Query returns up to topK matches for embedding ordered by descending score.
	// A topK <= 0 means 10. It returns ErrEmptyEmbedding for an empty embedding.
	Query(ctx context.Context, embedding []float32, topK int) ([]ScoreMatch, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a VectorStore from the given Options.
type Factory func(opts Options) (VectorStore, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateAdapterError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a VectorStore for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (VectorStore, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	vs, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: open %s: %w", adapter, err)
	}

	return vs, nil
}
