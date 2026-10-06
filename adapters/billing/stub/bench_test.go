package stub

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// benchAdapterSeq yields a unique adapter key per Register call so the
// registry benchmark never collides with an existing factory.
var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run.
func benchFreshAdapter() billing.Adapter {
	return billing.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

// BenchmarkCreateCustomer measures in-memory customer creation.
func BenchmarkCreateCustomer(b *testing.B) {
	bl := New()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
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

	for b.Loop() {
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

	for b.Loop() {
		if _, err := bl.GetInvoice(ctx, cus.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCancelSubscription measures cancelling a pre-created subscription.
func BenchmarkCancelSubscription(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		bl := New()

		cus, err := bl.CreateCustomer(ctx, "Ada", "ada@example.com", "")
		if err != nil {
			b.Fatal(err)
		}

		sub, err := bl.CreateSubscription(ctx, cus.ID, "plan-1", "")
		if err != nil {
			b.Fatal(err)
		}

		if err := bl.CancelSubscription(ctx, sub.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOpen measures option validation plus construction.
func BenchmarkOpen(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(billing.Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegister measures registering the stub factory into the billing
// registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := billing.Register(benchFreshAdapter(), Open); err != nil {
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
