package payment

import (
	"context"
	"errors"

	"github.com/zenta-dev/zever/core/idempotency"
)

// ErrMissingEventID is returned by HandleWebhook when replay dedupe is
// enabled but the verified event carries no provider event id. Such an event
// cannot be deduplicated safely, so it is rejected instead of being
// processed without protection.
var ErrMissingEventID = errors.New("payment: webhook event has no id")

// WebhookHandler processes a verified webhook event. Returning an error (or
// panicking) tells HandleWebhook the event was not processed, so the provider
// retry is accepted again.
type WebhookHandler func(ctx context.Context, ev Event) error

// WebhookHandling is an optional interface implemented by Payment backends
// that support at-least-once webhook processing. Use it via a type assertion:
//
//	if wh, ok := p.(payment.WebhookHandling); ok { ... }
//
// HandleWebhook verifies and decodes the payload like Payment.WebhookEvent,
// then reserves the event id, runs handler, and commits the id only when
// handler returns nil. On handler error or panic the reservation is released
// so a provider retry is processed. A repeat of a committed event fails with
// ErrDuplicateEvent; a delivery concurrent with an in-flight one fails with
// idempotency.ErrInProgress. Unlike Payment.WebhookEvent, which is
// at-most-once, an event is never lost to a failing handler.
type WebhookHandling interface {
	// HandleWebhook verifies raw against signature and runs handler once per
	// successfully processed event id.
	HandleWebhook(ctx context.Context, raw []byte, signature string, handler WebhookHandler) error
}

// ProcessWebhookOnce runs handler for ev under the idempotency reservation
// key, committing it only on success. It is the shared implementation behind
// WebhookHandling. A nil store runs handler without dedupe. With a store, an
// empty ev.ID fails with ErrMissingEventID. fingerprint binds the record to
// the delivered payload.
func ProcessWebhookOnce(ctx context.Context, store idempotency.Store, key string, ev Event, fingerprint []byte, handler WebhookHandler) error {
	if handler == nil {
		return errors.New("payment: nil webhook handler")
	}

	if store == nil {
		return handler(ctx, ev)
	}

	if ev.ID == "" {
		return ErrMissingEventID
	}

	outcome, err := store.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fingerprint, TTL: DefaultWebhookIdempotencyTTL})
	if err != nil {
		return err
	}

	if outcome.Replay {
		return DuplicateEventError{EventID: ev.ID}
	}

	committed := false

	defer func() {
		if !committed {
			// Release even if ctx was cancelled so retries are accepted.
			_ = store.Forget(context.WithoutCancel(ctx), key)
		}
	}()

	if err := handler(ctx, ev); err != nil {
		return err
	}

	if err := store.Complete(ctx, key, fingerprint, []byte{1}); err != nil {
		return err
	}

	committed = true

	return nil
}
