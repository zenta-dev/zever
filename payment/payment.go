package payment

import (
	"context"
	"fmt"
	"sync"
)

// Payment defines the payment-processing contract for payment backends.
// Implementations return the zero Result on error.
type Payment interface {
	// CreatePayment creates a payment from req. It returns the zero Result on error.
	CreatePayment(ctx context.Context, req Request) (Result, error)
	// Refund refunds amount (in minor units) against payment id.
	Refund(ctx context.Context, id string, amount int64) error
	// GetPayment fetches payment id. It returns the zero Result on error.
	GetPayment(ctx context.Context, id string) (Result, error)
	// WebhookEvent decodes and verifies a raw webhook payload. It returns the zero Event on error.
	WebhookEvent(ctx context.Context, raw []byte, signature string) (Event, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Payment from the given Options.
type Factory func(opts Options) (Payment, error)

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

// Open creates a Payment for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Payment, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	p, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("payment: open %s: %w", adapter, err)
	}

	return p, nil
}
