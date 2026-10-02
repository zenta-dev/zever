// Package paymenttest provides the conformance kit third-party payment adapters run to prove backend parity.
package paymenttest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// Conformance verifies factory-built payments implement the
// payment.Payment contract: open/register round-trip, Create/Get
// lifecycle, Refund partial/full with mismatch sentinels,
// invalid-request sentinels, fail-closed webhooks, and Close. Each
// subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network
// (live-provider adapters prove against recorded fakes in their own
// packages; the kit runs against the stub or sandbox wiring).
//
// Documented stub exemption: the stub performs zero signature
// verification by design, so WebhookEvent always fails closed with
// ErrInvalidSignature; the kit asserts that failure rather than a
// successful decode.
func Conformance(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("CreateGet", func(t *testing.T) { conformanceCreateGet(t, factory) })
	t.Run("InvalidRequest", func(t *testing.T) { conformanceInvalidRequest(t, factory) })
	t.Run("Refund", func(t *testing.T) { conformanceRefund(t, factory) })
	t.Run("RefundMismatch", func(t *testing.T) { conformanceRefundMismatch(t, factory) })
	t.Run("Webhook", func(t *testing.T) { conformanceWebhook(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := payment.Open(payment.Adapter("conformance-missing-adapter"), payment.Options{}); !errors.Is(err, payment.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := payment.Adapter("conformance-probe-payment")

	if err := payment.Register(probe, nil); !errors.Is(err, payment.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(payment.Options) (payment.Payment, error) {
		return nil, errors.New("paymenttest: probe factory must not run")
	}

	_ = payment.Register(probe, stub)

	if err := payment.Register(probe, stub); !errors.Is(err, payment.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceCreateGet(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 2500, Currency: "usd", Method: payment.MethodCard})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}

	if res.ID == "" {
		t.Fatal("CreatePayment() ID is empty")
	}

	if res.Amount != 2500 || res.Currency != "usd" {
		t.Errorf("CreatePayment() = %+v, want amount 2500 usd echoed", res)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if got.ID != res.ID {
		t.Errorf("GetPayment() ID = %q, want %q", got.ID, res.ID)
	}

	if _, err := p.GetPayment(ctx, "pay_missing"); !errors.Is(err, payment.ErrNotFound) {
		t.Errorf("GetPayment(missing) err = %v, want ErrNotFound", err)
	}
}

func conformanceInvalidRequest(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 0, Currency: "usd"}); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Errorf("CreatePayment(zero) err = %v, want ErrInvalidAmount", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: -5, Currency: "usd"}); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Errorf("CreatePayment(negative) err = %v, want ErrInvalidAmount", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: ""}); !errors.Is(err, payment.ErrMissingCurrency) {
		t.Errorf("CreatePayment(no currency) err = %v, want ErrMissingCurrency", err)
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "usd", Method: "bottle-caps"}); !errors.Is(err, payment.ErrUnsupportedMethod) {
		t.Errorf("CreatePayment(bad method) err = %v, want ErrUnsupportedMethod", err)
	}
}

func conformanceRefund(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}

	if err = p.Refund(ctx, res.ID, 400, "kit-refund-01"); err != nil {
		t.Fatalf("Refund(partial) error = %v", err)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if got.Status != payment.PaymentPartiallyRefunded {
		t.Errorf("Status = %q, want partially_refunded after partial refund", got.Status)
	}

	if err = p.Refund(ctx, res.ID, 600, "kit-refund-02"); err != nil {
		t.Fatalf("Refund(rest) error = %v", err)
	}

	got, err = p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if got.Status != payment.PaymentRefunded {
		t.Errorf("Status = %q, want refunded after full refund", got.Status)
	}
}

func conformanceRefundMismatch(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}

	if err := p.Refund(ctx, res.ID, 0, ""); !errors.Is(err, payment.ErrInvalidAmount) {
		t.Errorf("Refund(zero) err = %v, want ErrInvalidAmount", err)
	}

	if err := p.Refund(ctx, res.ID, 2000, ""); !errors.Is(err, payment.ErrAmountMismatch) {
		t.Errorf("Refund(over total) err = %v, want ErrAmountMismatch", err)
	}

	if err := p.Refund(ctx, "pay_missing", 100, ""); !errors.Is(err, payment.ErrNotFound) {
		t.Errorf("Refund(missing) err = %v, want ErrNotFound", err)
	}

	if err := p.Refund(ctx, "", 100, ""); err == nil {
		t.Error("Refund(empty id) = nil, want error")
	} else if !errors.Is(err, payment.ErrNotFound) && !errors.Is(err, payment.ErrMissingPaymentID) {
		t.Errorf("Refund(empty id) err = %v, want ErrNotFound or ErrMissingPaymentID", err)
	}
}

func conformanceWebhook(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	// The stub never verifies, so every webhook fails closed.
	if _, err := p.WebhookEvent(ctx, []byte(`{"type":"x"}`), "sig"); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Errorf("WebhookEvent(stub) err = %v, want ErrInvalidSignature", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	p := factory(t)

	if err := p.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
