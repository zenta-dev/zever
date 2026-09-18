package vectorstore

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// VectorStore defines the vector-similarity contract for vectorstore backends.
// Implementations return the zero value on error.
// Similarity is cosine similarity only; Score is higher for more similar vectors.
type VectorStore interface {
	// Upsert inserts or replaces vec. It returns ErrEmptyEmbedding for an empty embedding.
	Upsert(ctx context.Context, vec Vector) error
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

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateAdapterError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a VectorStore for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (VectorStore, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	vs, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: open %s: %w", adapter, err)
	}

	return vs, nil
}
