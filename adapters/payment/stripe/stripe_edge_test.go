package stripe

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
)

// fakeStore is a scriptable idempotency.Store for exercising the refund
// idempotency guard without a real backend.
type fakeStore struct {
	mu sync.Mutex

	beginOut    idempotency.Outcome
	beginErr    error
	completeErr error

	beginCalls    int
	completeCalls int
	forgetCalls   int
}

func (f *fakeStore) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beginCalls++
	return f.beginOut, f.beginErr
}

func (f *fakeStore) Complete(context.Context, string, []byte, []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completeCalls++
	return f.completeErr
}

func (f *fakeStore) Forget(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgetCalls++
	return nil
}

func (f *fakeStore) Close() error { return nil }

func (f *fakeStore) counts() (begin, complete, forget int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.beginCalls, f.completeCalls, f.forgetCalls
}

func driverWithStore(t *testing.T, f *fakeStore) (*driver, *fakeStripe) {
	t.Helper()
	fs := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, fs)
	p := mustOpen(t, srv, func(o *payment.Options) { o.Idempotency = f })
	d, ok := p.(*driver)
	if !ok {
		t.Fatalf("Open returned %T, want *driver", p)
	}
	return d, fs
}

func TestRefund_idempotencyReplay_skipsStripe(t *testing.T) {
	t.Parallel()

	store := &fakeStore{beginOut: idempotency.Outcome{Replay: true, Result: []byte{1}}}
	d, fs := driverWithStore(t, store)

	if err := d.Refund(t.Context(), "pi_test_123", 500, "refund-key"); err != nil {
		t.Fatalf("Refund(replay) = %v, want nil", err)
	}

	if _, complete, _ := store.counts(); complete != 0 {
		t.Errorf("Complete calls = %d, want 0 on replay", complete)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.refunds != 0 {
		t.Errorf("Stripe refunds = %d, want 0 on replay", fs.refunds)
	}
}

func TestRefund_idempotencyBeginError_wrapped(t *testing.T) {
	t.Parallel()

	boom := errors.New("store down")
	store := &fakeStore{beginErr: boom}
	d, fs := driverWithStore(t, store)

	err := d.Refund(t.Context(), "pi_test_123", 500, "refund-key")
	if !errors.Is(err, boom) {
		t.Errorf("Refund() = %v, want wrap of store error", err)
	}
	if !strings.HasPrefix(err.Error(), "stripe: refund:") {
		t.Errorf("Refund() = %v, want stripe: refund: prefix", err)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.refunds != 0 {
		t.Errorf("Stripe refunds = %d, want 0 after Begin error", fs.refunds)
	}
}

func TestRefund_idempotencyCompleteError_wrapped(t *testing.T) {
	t.Parallel()

	boom := errors.New("complete failed")
	store := &fakeStore{completeErr: boom}
	d, fs := driverWithStore(t, store)

	err := d.Refund(t.Context(), "pi_test_123", 500, "refund-key")
	if !errors.Is(err, boom) {
		t.Errorf("Refund() = %v, want wrap of complete error", err)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.refunds != 1 {
		t.Errorf("Stripe refunds = %d, want 1 (Stripe call succeeded)", fs.refunds)
	}
}

func TestRefund_idempotencyForgetOnStripeError(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	d, fs := driverWithStore(t, store)
	fs.setMode("refund402")

	err := d.Refund(t.Context(), "pi_test_123", 500, "refund-key")
	if err == nil {
		t.Fatal("Refund() = nil, want Stripe error")
	}
	if _, _, forget := store.counts(); forget != 1 {
		t.Errorf("Forget calls = %d, want 1 after Stripe failure", forget)
	}
}

func TestRefund_emptyKey_skipsStore(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	d, _ := driverWithStore(t, store)

	if err := d.Refund(t.Context(), "pi_test_123", 500, ""); err != nil {
		t.Fatalf("Refund() = %v, want nil", err)
	}
	if begin, complete, forget := store.counts(); begin != 0 || complete != 0 || forget != 0 {
		t.Errorf("store calls = (%d,%d,%d), want none with empty key", begin, complete, forget)
	}
}

func TestCreatePayment_emptyMeta_noPanic(t *testing.T) {
	t.Parallel()

	fs := &fakeStripe{piID: "pi_test_123"}
	srv := newFake(t, fs)
	p := mustOpen(t, srv, nil)

	if _, err := p.CreatePayment(t.Context(), payment.Request{Amount: 100, Currency: "usd", Meta: map[string]string{}}); err != nil {
		t.Fatalf("CreatePayment(empty meta) = %v, want nil", err)
	}
}
