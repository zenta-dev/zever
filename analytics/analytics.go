package analytics

import (
	"context"
	"fmt"
	"sync"
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

// Open creates an Analytics for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Analytics, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	a, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("analytics: open %s: %w", adapter, err)
	}

	return a, nil
}
