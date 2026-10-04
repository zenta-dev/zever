package single

import (
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

// newBenchTenant opens the fixed-ID adapter for benchmarks.
func newBenchTenant(b *testing.B) tenant.Tenant {
	b.Helper()

	tn, err := New(tenant.Options{ID: "acme"})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = tn.Close() })

	return tn
}

// BenchmarkResolve measures the fixed-ID Resolve fast path (no map lookup,
// no allocation beyond the return value).
func BenchmarkResolve(b *testing.B) {
	tn := newBenchTenant(b)
	ctx := b.Context()
	meta := map[string]string{"x": "y"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := tn.Resolve(ctx, meta); err != nil {
			b.Fatalf("Resolve(): %v", err)
		}
	}
}

// BenchmarkResolveParallel measures Resolve throughput under concurrent load;
// the adapter is immutable and therefore safe to share.
func BenchmarkResolveParallel(b *testing.B) {
	tn := newBenchTenant(b)
	ctx := b.Context()
	meta := map[string]string{"x": "y"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := tn.Resolve(ctx, meta); err != nil {
				b.Errorf("Resolve(): %v", err)
				return
			}
		}
	})
}

// BenchmarkScoped measures building a tenant-scoped context.
func BenchmarkScoped(b *testing.B) {
	tn := newBenchTenant(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := tn.Scoped(ctx, "acme"); err != nil {
			b.Fatalf("Scoped(): %v", err)
		}
	}
}
