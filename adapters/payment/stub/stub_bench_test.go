package stub

import (
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

func benchPayment(b *testing.B) payment.Payment {
	b.Helper()
	p, err := New(payment.Options{AutoApprove: true})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })
	return p
}

// BenchmarkCreatePayment measures the ledger-insert hot path.
func BenchmarkCreatePayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 100, Currency: "USD", Method: payment.MethodCard}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.CreatePayment(ctx, req); err != nil {
			b.Fatalf("CreatePayment() = %v", err)
		}
	}
}

// BenchmarkCreatePaymentParallel measures concurrent ledger inserts.
func BenchmarkCreatePaymentParallel(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 100, Currency: "USD", Method: payment.MethodCard}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := p.CreatePayment(ctx, req); err != nil {
				b.Errorf("CreatePayment() = %v", err)
				return
			}
		}
	})
}

// BenchmarkGetPayment measures the ledger-read hot path.
func BenchmarkGetPayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	res, err := p.CreatePayment(ctx, payment.Request{Amount: 100, Currency: "USD"})
	if err != nil {
		b.Fatalf("CreatePayment() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.GetPayment(ctx, res.ID); err != nil {
			b.Fatalf("GetPayment() = %v", err)
		}
	}
}

// BenchmarkRefund measures the cumulative-refund hot path against a payment
// large enough that a single-unit refund never exhausts it.
func BenchmarkRefund(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1 << 62, Currency: "USD"})
	if err != nil {
		b.Fatalf("CreatePayment() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.Refund(ctx, res.ID, 1, ""); err != nil {
			b.Fatalf("Refund() = %v", err)
		}
	}
}
