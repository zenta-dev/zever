// Package paymenttest provides the conformance kit third-party payment adapters run to prove backend parity.
package paymenttest

import (
	"context"
	"errors"
	"fmt"
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

	if err := checkOpenRegister(); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the open/register round-trip against the
// shared registry. It returns a descriptive error on the first
// contract violation so unit tests can drive every branch.
func checkOpenRegister() error {
	if _, err := payment.Open(payment.Adapter("conformance-missing-adapter"), payment.Options{}); !errors.Is(err, payment.ErrUnknownAdapter) {
		return fmt.Errorf("Open(missing) err = %w, want ErrUnknownAdapter", err)
	}

	probe := payment.Adapter("conformance-probe-payment")

	if err := payment.Register(probe, nil); !errors.Is(err, payment.ErrNilFactory) {
		return fmt.Errorf("Register(nil) err = %w, want ErrNilFactory", err)
	}

	stub := func(payment.Options) (payment.Payment, error) {
		return nil, errors.New("paymenttest: probe factory must not run")
	}

	_ = payment.Register(probe, stub)

	if err := payment.Register(probe, stub); !errors.Is(err, payment.ErrDuplicate) {
		return fmt.Errorf("Register(duplicate) err = %w, want ErrDuplicate", err)
	}

	return nil
}

func conformanceCreateGet(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkCreateGet(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkCreateGet proves Create echoes amount/currency and Get
// round-trips the ID while missing IDs surface ErrNotFound.
func checkCreateGet(ctx context.Context, p payment.Payment) error {
	res, err := p.CreatePayment(ctx, payment.Request{Amount: 2500, Currency: "usd", Method: payment.MethodCard})
	if err != nil {
		return fmt.Errorf("CreatePayment() error = %w", err)
	}

	if res.ID == "" {
		return errors.New("CreatePayment() ID is empty")
	}

	if res.Amount != 2500 || res.Currency != "usd" {
		return fmt.Errorf("CreatePayment() = %+v, want amount 2500 usd echoed", res)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		return fmt.Errorf("GetPayment() error = %w", err)
	}

	if got.ID != res.ID {
		return fmt.Errorf("GetPayment() ID = %q, want %q", got.ID, res.ID)
	}

	if _, err := p.GetPayment(ctx, "pay_missing"); !errors.Is(err, payment.ErrNotFound) {
		return fmt.Errorf("GetPayment(missing) err = %w, want ErrNotFound", err)
	}

	return nil
}

func conformanceInvalidRequest(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkInvalidRequest(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidRequest proves malformed requests surface the
// invalid-request sentinels.
func checkInvalidRequest(ctx context.Context, p payment.Payment) error {
	var errs []error

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 0, Currency: "usd"}); !errors.Is(err, payment.ErrInvalidAmount) {
		errs = append(errs, fmt.Errorf("CreatePayment(zero) err = %w, want ErrInvalidAmount", err))
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: -5, Currency: "usd"}); !errors.Is(err, payment.ErrInvalidAmount) {
		errs = append(errs, fmt.Errorf("CreatePayment(negative) err = %w, want ErrInvalidAmount", err))
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: ""}); !errors.Is(err, payment.ErrMissingCurrency) {
		errs = append(errs, fmt.Errorf("CreatePayment(no currency) err = %w, want ErrMissingCurrency", err))
	}

	if _, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "usd", Method: "bottle-caps"}); !errors.Is(err, payment.ErrUnsupportedMethod) {
		errs = append(errs, fmt.Errorf("CreatePayment(bad method) err = %w, want ErrUnsupportedMethod", err))
	}

	return errors.Join(errs...)
}

func conformanceRefund(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkRefund(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkRefund proves a partial refund flips status to
// partially_refunded and completing it flips to refunded.
func checkRefund(ctx context.Context, p payment.Payment) error {
	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		return fmt.Errorf("CreatePayment() error = %w", err)
	}

	if err = p.Refund(ctx, res.ID, 400, "kit-refund-01"); err != nil {
		return fmt.Errorf("Refund(partial) error = %w", err)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		return fmt.Errorf("GetPayment() error = %w", err)
	}

	if got.Status != payment.PaymentPartiallyRefunded {
		return fmt.Errorf("Status = %q, want partially_refunded after partial refund", got.Status) //nolint:staticcheck // kit output mirrors the Status field name.
	}

	if err = p.Refund(ctx, res.ID, 600, "kit-refund-02"); err != nil {
		return fmt.Errorf("Refund(rest) error = %w", err)
	}

	got, err = p.GetPayment(ctx, res.ID)
	if err != nil {
		return fmt.Errorf("GetPayment() error = %w", err)
	}

	if got.Status != payment.PaymentRefunded {
		return fmt.Errorf("Status = %q, want refunded after full refund", got.Status) //nolint:staticcheck // kit output mirrors the Status field name.
	}

	return nil
}

func conformanceRefundMismatch(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkRefundMismatch(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkRefundMismatch proves bad refund inputs surface the amount and
// not-found sentinels. Empty IDs are accepted as either ErrNotFound
// or ErrMissingPaymentID.
func checkRefundMismatch(ctx context.Context, p payment.Payment) error {
	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		return fmt.Errorf("CreatePayment() error = %w", err)
	}

	var errs []error

	if err := p.Refund(ctx, res.ID, 0, ""); !errors.Is(err, payment.ErrInvalidAmount) {
		errs = append(errs, fmt.Errorf("Refund(zero) err = %w, want ErrInvalidAmount", err))
	}

	if err := p.Refund(ctx, res.ID, 2000, ""); !errors.Is(err, payment.ErrAmountMismatch) {
		errs = append(errs, fmt.Errorf("Refund(over total) err = %w, want ErrAmountMismatch", err))
	}

	if err := p.Refund(ctx, "pay_missing", 100, ""); !errors.Is(err, payment.ErrNotFound) {
		errs = append(errs, fmt.Errorf("Refund(missing) err = %w, want ErrNotFound", err))
	}

	if err := p.Refund(ctx, "", 100, ""); err == nil {
		errs = append(errs, errors.New("Refund(empty id) = nil, want error"))
	} else if !errors.Is(err, payment.ErrNotFound) && !errors.Is(err, payment.ErrMissingPaymentID) {
		errs = append(errs, fmt.Errorf("Refund(empty id) err = %w, want ErrNotFound or ErrMissingPaymentID", err))
	}

	return errors.Join(errs...)
}

func conformanceWebhook(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkWebhook(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkWebhook proves the fail-closed stub contract: every webhook
// fails with ErrInvalidSignature. The stub never verifies, so every
// webhook fails closed.
func checkWebhook(ctx context.Context, p payment.Payment) error {
	if _, err := p.WebhookEvent(ctx, []byte(`{"type":"x"}`), "sig"); !errors.Is(err, payment.ErrInvalidSignature) {
		return fmt.Errorf("WebhookEvent(stub) err = %w, want ErrInvalidSignature", err)
	}

	return nil
}

func conformanceClose(t *testing.T, factory func(t *testing.T) payment.Payment) {
	t.Helper()

	if err := checkClose(factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent.
func checkClose(p payment.Payment) error {
	if err := p.Close(); err != nil {
		return fmt.Errorf("Close() error = %w", err)
	}

	if err := p.Close(); err != nil {
		return fmt.Errorf("Close() second error = %w, want nil", err)
	}

	return nil
}
