package stub

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

func openCents(tb testing.TB) payment.Payment {
	tb.Helper()

	p, err := New(payment.Options{AutoApprove: true})
	if err != nil {
		tb.Fatalf("New: %v", err)
	}

	return p
}

// TestMinorUnitsRoundTrip proves the stub stores minor units verbatim:
// creating 1000 returns and fetches 1000 with no scaling.
func TestMinorUnitsRoundTrip(t *testing.T) {
	t.Parallel()

	p := openCents(t)
	ctx := t.Context()

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if res.Amount != 1000 {
		t.Fatalf("CreatePayment amount = %d, want 1000 minor units", res.Amount)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}

	if got.Amount != res.Amount {
		t.Fatalf("GetPayment amount = %d, want created %d", got.Amount, res.Amount)
	}
}

// TestRefundSymmetry proves refunds draw from the same minor-unit total:
// 400 + 600 of 1000 fully refunds, and 1 more fails against the remainder.
func TestRefundSymmetry(t *testing.T) {
	t.Parallel()

	p := openCents(t)
	ctx := t.Context()

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if rerr := p.Refund(ctx, res.ID, 400, ""); rerr != nil {
		t.Fatalf("partial Refund: %v", rerr)
	}

	if rerr := p.Refund(ctx, res.ID, 600, ""); rerr != nil {
		t.Fatalf("final Refund: %v", rerr)
	}

	got, err := p.GetPayment(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}

	if got.Status != payment.PaymentRefunded {
		t.Fatalf("status = %q, want refunded", got.Status)
	}

	var over payment.AmountMismatchError

	if err := p.Refund(ctx, res.ID, 1, ""); !errors.As(err, &over) || over.Expected != 0 || over.Actual != 1 {
		t.Fatalf("expected exhausted mismatch 0/1, got %v", err)
	}
}
