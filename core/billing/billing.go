package billing

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// Billing defines the subscription and invoice contract for billing backends.
// Implementations return the zero Customer, Subscription, or Invoice on error.
type Billing interface {
	// CreateCustomer creates a customer from name and email. idempotencyKey,
	// when non-empty, is passed to the backend's own idempotency mechanism
	// (e.g. Stripe's Idempotency-Key header) so a caller-side retry of the
	// same logical attempt (same key) does not create a second customer.
	// Generate the key once per logical attempt and reuse it across
	// retries of that attempt, never derive it from name/email alone: two
	// distinct customers can legitimately share those. Backends without an
	// equivalent mechanism ignore it. It returns the zero Customer on error.
	CreateCustomer(ctx context.Context, name, email, idempotencyKey string) (Customer, error)
	// CreateSubscription creates a subscription for customerID on planID.
	// idempotencyKey has the same contract as CreateCustomer's: non-empty
	// and reused across retries of one logical attempt, it prevents a
	// retried request from creating a second, separately-billed
	// subscription. Do not derive it from customerID/planID alone: a
	// customer legitimately resubscribing to the same plan after
	// cancellation must not be deduped against their prior subscription.
	// It returns the zero Subscription on error.
	CreateSubscription(ctx context.Context, customerID, planID, idempotencyKey string) (Subscription, error)
	// CancelSubscription cancels subscription id.
	CancelSubscription(ctx context.Context, id string) error
	// GetInvoice fetches the invoice for customerID. It returns the zero Invoice on error.
	GetInvoice(ctx context.Context, customerID string) (Invoice, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Billing from the given Options.
type Factory func(opts Options) (Billing, error)

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

// Open creates a Billing for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Billing, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	b, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("billing: open %s: %w", adapter, err)
	}

	return b, nil
}
