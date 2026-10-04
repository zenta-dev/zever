package stub

import (
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// BenchmarkCreateCustomer measures in-memory customer creation.
func BenchmarkCreateCustomer(b *testing.B) {
	bl := New()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", ""); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateSubscription measures in-memory subscription creation over a
// pre-created customer.
func BenchmarkCreateSubscription(b *testing.B) {
	bl := New()
	ctx := b.Context()

	cus, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", "")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := bl.CreateSubscription(ctx, cus.ID, "plan-1", ""); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetInvoice measures invoice lookup over a pre-populated store.
func BenchmarkGetInvoice(b *testing.B) {
	bl := New()
	ctx := b.Context()

	cus, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", "")
	if err != nil {
		b.Fatal(err)
	}

	if _, err := bl.CreateSubscription(ctx, cus.ID, "plan-1", ""); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := bl.GetInvoice(ctx, cus.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateCustomerParallel measures customer creation under concurrent
// load; the stub guards its maps with a mutex.
func BenchmarkCreateCustomerParallel(b *testing.B) {
	bl := New()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", ""); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

var _ billing.Billing = New()
