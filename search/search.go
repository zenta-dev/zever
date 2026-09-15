package search

import (
	"context"
	"fmt"
	"sync"
)

// Search defines the full-text search contract for search backends.
// Implementations return the zero value on error.
type Search interface {
	// Index adds or replaces doc in its index. It returns an error on failure.
	Index(ctx context.Context, doc Document) error
	// Delete removes the document with id. Backends without global ID lookup may require known ids. It returns an error on failure.
	Delete(ctx context.Context, id string) error
	// Search runs query with opts and returns ranked hits. An explicit index filter required. It returns Result{} on error.
	Search(ctx context.Context, query string, opts QueryOptions) (Result, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Search from the given Options.
type Factory func(o Options) (Search, error)

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

// Open creates a Search for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Search, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("search: open %s: %w", adapter, err)
	}

	return s, nil
}
