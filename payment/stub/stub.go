package stub

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/zenta-dev/zever/payment"
)

// driver is the in-memory stub payment backend.
type driver struct {
	// mu guards ledger and refunded.
	mu sync.RWMutex
	// ledger stores created payments by ID.
	ledger map[string]payment.Result
	// refunded tracks cumulative refunded amounts by payment ID.
	refunded map[string]int64
	// autoApprove marks new payments succeeded instead of pending.
	autoApprove bool
	// seq generates stub_N payment IDs.
	seq atomic.Int64
}

// New creates an in-memory stub payment backend.
func New(autoApprove bool) payment.Payment {
	return &driver{
		ledger:      make(map[string]payment.Result),
		refunded:    make(map[string]int64),
		autoApprove: autoApprove,
	}
}

// Open validates o then returns a stub backend honoring AutoApprove.
func Open(o payment.Options) (payment.Payment, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("stub: %w", err)
	}

	return New(o.AutoApprove), nil
}

// CreatePayment stores a new stub payment and returns it.
func (d *driver) CreatePayment(_ context.Context, req payment.Request) (payment.Result, error) {
	if req.Amount <= 0 {
		return payment.Result{}, fmt.Errorf("stub: create: %w", payment.ErrInvalidAmount)
	}

	if req.Currency == "" {
		return payment.Result{}, fmt.Errorf("stub: create: %w", payment.ErrMissingCurrency)
	}

	if req.Method != "" && req.Method != payment.MethodCard && req.Method != payment.MethodBankTransfer {
		methodErr := error(&payment.UnsupportedMethodError{Method: req.Method})
		return payment.Result{}, fmt.Errorf("stub: create: %w", methodErr)
	}

	id := "stub_" + strconv.FormatInt(d.seq.Add(1), 10)

	status := payment.PaymentPending
	if d.autoApprove {
		status = payment.PaymentSucceeded
	}

	res := payment.Result{
		ID:       id,
		Status:   status,
		Amount:   req.Amount,
		Currency: req.Currency,
	}

	d.mu.Lock()
	d.ledger[id] = res
	d.mu.Unlock()

	return res, nil
}

// Refund applies amount against payment id up to the cumulative total.
func (d *driver) Refund(_ context.Context, id string, amount int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	res, ok := d.ledger[id]
	if !ok {
		notFound := error(&payment.NotFoundError{PaymentID: id})
		return fmt.Errorf("stub: refund: %w", notFound)
	}

	if amount <= 0 {
		return fmt.Errorf("stub: refund: %w", payment.ErrInvalidAmount)
	}

	if amount > res.Amount {
		mismatch := error(&payment.AmountMismatchError{Expected: res.Amount, Actual: amount})
		return fmt.Errorf("stub: refund: %w", mismatch)
	}

	if d.refunded[id]+amount > res.Amount {
		mismatch := error(&payment.AmountMismatchError{Expected: res.Amount - d.refunded[id], Actual: amount})
		return fmt.Errorf("stub: refund: %w", mismatch)
	}

	d.refunded[id] += amount
	if d.refunded[id] < res.Amount {
		res.Status = payment.PaymentPartiallyRefunded
	} else {
		res.Status = payment.PaymentRefunded
	}

	d.ledger[id] = res

	return nil
}

// GetPayment fetches payment id from the ledger.
func (d *driver) GetPayment(_ context.Context, id string) (payment.Result, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	res, ok := d.ledger[id]
	if !ok {
		notFound := error(&payment.NotFoundError{PaymentID: id})
		return payment.Result{}, fmt.Errorf("stub: get: %w", notFound)
	}

	return res, nil
}

// WebhookEvent returns a static test-only event without verification.
func (d *driver) WebhookEvent(_ context.Context, _ []byte, _ string) (payment.Event, error) {
	// Static test-only event; no payload inspection or signature check.
	return payment.Event{
		Type:   payment.EventType("stub.event"),
		Object: payment.Result{},
	}, nil
}

// Close releases backend resources.
func (d *driver) Close() error {
	return nil
}
