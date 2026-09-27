package analytics

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// Analytics defines the event-tracking contract for analytics backends.
type Analytics interface {
	// Track records an event with optional properties.
	Track(ctx context.Context, event string, properties map[string]any) error
	// Identify associates traits with a user ID.
	Identify(ctx context.Context, userID string, traits map[string]any) error
	// Group associates a user with a group and its traits.
	Group(ctx context.Context, userID string, groupID string, traits map[string]any) error
	// Close flushes buffered events and releases backend resources.
	// Callers must call Close before process exit to avoid dropping events.
	Close() error
}

// Factory creates an Analytics from the given Options.
type Factory func(opts Options) (Analytics, error)

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

// Open creates an Analytics for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Analytics, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	a, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("analytics: open %s: %w", adapter, err)
	}

	return a, nil
}
