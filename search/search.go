package search

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// Search defines the full-text search contract for search backends.
// Implementations return the zero value on error.
type Search interface {
	// Index adds or replaces doc in its index. It returns an error on failure.
	Index(ctx context.Context, doc Document) error
	// IndexBatch adds or replaces all of docs in their indexes. It returns an error on failure.
	IndexBatch(ctx context.Context, docs []Document) error
	// Delete removes the document with id. Backends without global ID lookup may require known ids. It returns an error on failure.
	Delete(ctx context.Context, id string) error
	// Search runs query with opts and returns ranked hits. An explicit index filter required. It returns Result{} on error.
	Search(ctx context.Context, query string, opts QueryOptions) (Result, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Search from the given Options.
type Factory func(o Options) (Search, error)

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

// Open creates a Search for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Search, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("search: open %s: %w", adapter, err)
	}

	return s, nil
}
