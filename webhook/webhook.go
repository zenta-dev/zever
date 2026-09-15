package webhook

import (
	"context"
	"fmt"
	"sync"
)

// Webhook defines the core operations for webhook delivery.
// It handles subscription management and event fan-out.
type Webhook interface {
	// Register subscribes target to event, securing delivery with secret.
	// Implementations must never log secret.
	Register(ctx context.Context, event, target, secret string) error

	// Unregister removes the subscription of target from event.
	// It returns ErrNotFound when no such subscription exists.
	Unregister(ctx context.Context, event, target string) error

	// Deliver fans payload out to every target subscribed to event.
	Deliver(ctx context.Context, event string, payload []byte) error

	// Close shuts down the webhook and releases associated resources.
	Close() error
}

// Factory creates a Webhook from the given Options.
type Factory func(o Options) (Webhook, error)

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

// Open creates a Webhook for adapter using the registered Factory and opts.
// Options are validated first, so misconfiguration fails before any factory runs.
// Open returns a nil Webhook on any error; callers must not use the first
// result when err is non-nil.
func Open(adapter Adapter, opts Options) (Webhook, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("webhook: open %s: %w", adapter, err)
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	w, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("webhook: open %s: %w", adapter, err)
	}

	return w, nil
}
