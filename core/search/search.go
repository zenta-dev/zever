package search

import (
	"context"
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
type Factory func(opts Options) (Search, error)
