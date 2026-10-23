package webhook

import (
	"context"
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
