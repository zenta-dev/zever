package container

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
)

// paymentWiringSeq mints per-invocation adapter names so repeated runs in
// one process (go test -count=N) never collide in the global registry.
var paymentWiringSeq atomic.Uint64

// capturePaymentFactory records the options Payment resolution passed in.
type capturePaymentFactory struct {
	got payment.Options
}

func (f *capturePaymentFactory) open(o payment.Options) (payment.Payment, error) {
	f.got = o

	return &fakePayment{}, nil
}

// stubIdempotencyStore is a no-op idempotency.Store for wiring tests.
type stubIdempotencyStore struct{}

func (s *stubIdempotencyStore) Begin(_ context.Context, _ string, _ idempotency.BeginOptions) (idempotency.Outcome, error) {
	return idempotency.Outcome{}, nil
}

func (s *stubIdempotencyStore) Complete(_ context.Context, _ string, _, _ []byte) error {
	return nil
}

func (s *stubIdempotencyStore) Forget(_ context.Context, _ string) error { return nil }
func (s *stubIdempotencyStore) Close() error                             { return nil }

// registerCapturePayment wires a unique payment adapter that records its options.
func registerCapturePayment(t *testing.T) (payment.Adapter, *capturePaymentFactory) {
	t.Helper()

	registerFakeAdapters()

	name := payment.Adapter("pay-capture-" + strconv.FormatUint(paymentWiringSeq.Add(1), 10))
	factory := &capturePaymentFactory{}

	if err := payment.Register(name, factory.open); err != nil {
		t.Fatalf("payment.Register(%q) err = %v", name, err)
	}

	return name, factory
}

func TestPayment_reusesIdempotencyService(t *testing.T) {
	t.Parallel()

	name, factory := registerCapturePayment(t)

	cfg := testConfig(t)
	cfg.Payment.Adapter = string(name)

	c := New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	if _, err := c.Payment(); err != nil {
		t.Fatalf("Payment() err = %v", err)
	}

	if factory.got.Idempotency == nil {
		t.Fatal("Payment options Idempotency = nil, want the resolved idempotency service")
	}

	want, err := c.Idempotency()
	if err != nil {
		t.Fatalf("Idempotency() err = %v", err)
	}

	if factory.got.Idempotency != want {
		t.Fatal("Payment options Idempotency is not the container idempotency instance")
	}
}

func TestPayment_keepsExplicitIdempotencyStore(t *testing.T) {
	t.Parallel()

	name, factory := registerCapturePayment(t)

	cfg := testConfig(t)
	cfg.Payment.Adapter = string(name)

	explicit := &stubIdempotencyStore{}
	cfg.Payment.Options.Idempotency = explicit

	c := New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	if _, err := c.Payment(); err != nil {
		t.Fatalf("Payment() err = %v", err)
	}

	if factory.got.Idempotency != idempotency.Store(explicit) {
		t.Fatal("Payment options Idempotency was overridden, want the explicit store kept")
	}
}

func TestPayment_unresolvableIdempotencyStillResolves(t *testing.T) {
	t.Parallel()

	name, factory := registerCapturePayment(t)

	cfg := testConfig(t)
	cfg.Payment.Adapter = string(name)
	cfg.Idempotency.Adapter = "idem-bogus-" + strconv.FormatUint(paymentWiringSeq.Add(1), 10)

	c := New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	if _, err := c.Payment(); err != nil {
		t.Fatalf("Payment() err = %v, want resolution without idempotency", err)
	}

	if factory.got.Idempotency != nil {
		t.Fatal("Payment options Idempotency != nil, want no store when idempotency is unresolvable")
	}
}
