package billingtest

import (
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// healthyCheckBilling returns a stub satisfying every check so benchmarks
// measure the assertion path, not error construction.
func healthyCheckBilling() *stubBilling {
	return healthyStubBilling()
}

// BenchmarkCheckLifecycle measures the lifecycle assertion hot path.
func BenchmarkCheckLifecycle(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkLifecycle(ctx, healthyCheckBilling()); err != nil {
			b.Fatalf("checkLifecycle() err = %v", err)
		}
	}
}

// BenchmarkCheckCancel measures the cancel assertion hot path.
func BenchmarkCheckCancel(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkCancel(ctx, healthyCheckBilling()); err != nil {
			b.Fatalf("checkCancel() err = %v", err)
		}
	}
}

// BenchmarkCheckNotFound measures the not-found assertion hot path.
func BenchmarkCheckNotFound(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stub := healthyCheckBilling()
		stub.createSubErr = billing.ErrNotFound
		stub.cancelErr = billing.ErrNotFound
		stub.getInvoiceErr = billing.ErrNotFound

		if err := checkNotFound(ctx, stub); err != nil {
			b.Fatalf("checkNotFound() err = %v", err)
		}
	}
}

// BenchmarkCheckMissingIDs measures the missing-ID assertion hot path.
func BenchmarkCheckMissingIDs(b *testing.B) {
	ctx := b.Context()
	stub := &missingIDStubBilling{stub: healthyCheckBilling()}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkMissingIDs(ctx, stub); err != nil {
			b.Fatalf("checkMissingIDs() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(healthyCheckBilling()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}
