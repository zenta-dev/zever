package stripe

import (
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// benchBilling opens a Stripe adapter against the default in-process mux.
func benchBilling(b *testing.B) (billing.Billing, func()) {
	b.Helper()

	srv := httptest.NewServer(newDefaultMux(nil))
	bl := openWithServer(b, srv)

	return bl, srv.Close
}

// BenchmarkCreateCustomer measures a customer-create round trip through the
// in-process Stripe API stub.
func BenchmarkCreateCustomer(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", ""); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateSubscription measures a subscription-create round trip.
func BenchmarkCreateSubscription(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := bl.CreateSubscription(ctx, "cus_123", "price_123", ""); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCancelSubscription measures a subscription-cancel round trip.
func BenchmarkCancelSubscription(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := bl.CancelSubscription(ctx, "sub_1"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetInvoice measures invoice listing and filtering.
func BenchmarkGetInvoice(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := bl.GetInvoice(ctx, "cus_123"); err != nil {
			b.Fatal(err)
		}
	}
}
