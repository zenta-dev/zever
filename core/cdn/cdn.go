package cdn

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// CDN purges cached content from a CDN provider's edge network.
type CDN interface {
	// Purge invalidates cached content matching req.
	Purge(ctx context.Context, req PurgeRequest) error
	// Close releases any resources held by the CDN client.
	Close(ctx context.Context) error
	// Name returns the canonical adapter name.
	Name() string
}

// PurgeRequest describes what to invalidate. A zero value is a no-op
// and fails validation in Purge.
type PurgeRequest struct {
	// URLs lists full URLs to purge.
	URLs []string
	// Tags lists cache tags to purge.
	Tags []string
	// All purges the entire zone when true.
	All bool
}

// Factory creates a CDN from the given Options.
type Factory func(opts Options) (CDN, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("cdn: %w for adapter %s", ErrNilFactory, adapter)
	}
	return factories.Register(adapter, factory)
}

// Open creates a CDN for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (CDN, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}
	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("cdn: open %s: %w", adapter, err)
	}
	return c, nil
}
