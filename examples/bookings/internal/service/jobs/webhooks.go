package jobs

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/webhook"
)

// Webhook events fanned out by the bookings jobs.
const (
	EventCreated   = "booking.created"
	EventCancelled = "booking.cancelled"
)

// Environment names for the two demo webhook targets and their per-target
// secrets. Secrets come from the environment only and are never stored in
// zever.yaml; empty target URLs are skipped so the worker runs without them.
const (
	EnvCreatedTarget   = "BOOKINGS_WEBHOOK_CREATED_TARGET"
	EnvCreatedSecret   = "BOOKINGS_WEBHOOK_CREATED_SECRET" //nolint:gosec // environment variable name, not a credential.
	EnvCancelledTarget = "BOOKINGS_WEBHOOK_CANCELLED_TARGET"
	EnvCancelledSecret = "BOOKINGS_WEBHOOK_CANCELLED_SECRET" //nolint:gosec // environment variable name, not a credential.
)

// RegisterDemoTargets subscribes the configured demo targets: EventCreated
// and EventCancelled. lookup reads names (os.Getenv in production); pairs
// with an empty target are skipped.
func RegisterDemoTargets(ctx context.Context, wh webhook.Webhook, lookup func(string) string) error {
	pairs := []struct {
		event, target, secret string
	}{
		{EventCreated, lookup(EnvCreatedTarget), lookup(EnvCreatedSecret)},
		{EventCancelled, lookup(EnvCancelledTarget), lookup(EnvCancelledSecret)},
	}
	for _, p := range pairs {
		if p.target == "" {
			continue
		}
		if err := wh.Register(ctx, p.event, p.target, p.secret); err != nil {
			return fmt.Errorf("[jobs] register webhook %s: %w", p.event, err)
		}
	}
	return nil
}
