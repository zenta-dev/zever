package billing

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// Billing defines the subscription and invoice contract for billing backends.
// Implementations return the zero Customer, Subscription, or Invoice on error.
type Billing interface {
	// CreateCustomer creates a customer from name and email. It returns the zero Customer on error.
	CreateCustomer(ctx context.Context, name string, email string) (Customer, error)
	// CreateSubscription creates a subscription for customerID on planID. It returns the zero Subscription on error.
	CreateSubscription(ctx context.Context, customerID string, planID string) (Subscription, error)
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
