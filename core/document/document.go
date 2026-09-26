package document

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// Document defines the document-rendering contract for document backends.
// Implementations return nil output on error.
type Document interface {
	// Render renders source into format. It returns nil output on error.
	Render(ctx context.Context, source []byte, format OutputFormat) ([]byte, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Document from the given Options.
type Factory func(opts Options) (Document, error)

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

// Open creates a Document for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Document, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	d, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("document: open %s: %w", adapter, err)
	}

	return d, nil
}
