package payment

import (
	"context"
)

// Payment defines the payment-processing contract for payment backends.
// Implementations return the zero Result on error.
type Payment interface {
	// CreatePayment creates a payment from req. It returns the zero Result on error.
	CreatePayment(ctx context.Context, req Request) (Result, error)
	// Refund refunds amount (in minor units) against payment id.
	// key is the idempotency key; empty skips the idempotency guard.
	Refund(ctx context.Context, id string, amount int64, key string) error
	// GetPayment fetches payment id. It returns the zero Result on error.
	GetPayment(ctx context.Context, id string) (Result, error)
	// WebhookEvent decodes and verifies a raw webhook payload. It returns the zero Event on error.
	WebhookEvent(ctx context.Context, raw []byte, signature string) (Event, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Payment from the given Options.
type Factory func(opts Options) (Payment, error)
