package paddle

import (
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// benchBilling opens a Paddle adapter against an in-process flow server.
func benchBilling(b *testing.B) (billing.Billing, func()) {
	b.Helper()

	srv := httptest.NewServer(flowMux(b))
	bl := openTest(b, srv.URL)

	return bl, srv.Close
}

// BenchmarkCreateCustomer measures a customer-create round trip through the
// in-process Paddle flow server.
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
		if _, err := bl.CreateSubscription(ctx, "ctm_123", "pri_123", ""); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCancelSubscription measures a subscription-cancel round trip
// (transaction status patch).
func BenchmarkCancelSubscription(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := bl.CancelSubscription(ctx, "txn_123"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetInvoice measures invoice retrieval and total parsing.
func BenchmarkGetInvoice(b *testing.B) {
	bl, stop := benchBilling(b)
	defer stop()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := bl.GetInvoice(ctx, "ctm_123"); err != nil {
			b.Fatal(err)
		}
	}
}
