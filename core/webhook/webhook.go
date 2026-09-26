package webhook

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
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
type Factory func(opts Options) (Webhook, error)

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

// Open creates a Webhook for adapter using the registered Factory and opts.
// Options are validated first, so misconfiguration fails before any factory runs.
// Open returns a nil Webhook on any error; callers must not use the first
// result when err is non-nil.
func Open(adapter Adapter, opts Options) (Webhook, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	w, err := factory(opts)
	if err != nil {
		return nil, err
	}

	return w, nil
}
