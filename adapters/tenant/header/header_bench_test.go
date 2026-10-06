package header

import (
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

// newBenchHeader opens a header adapter with subdomain fallback configured.
func newBenchHeader(b *testing.B) tenant.Tenant {
	b.Helper()

	tn, err := New(tenant.Options{
		Header:         "X-Tenant-ID",
		SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`,
	})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = tn.Close() })

	return tn
}

// BenchmarkResolveHeader measures resolution straight from the tenant header,
// the common trusted-gateway path.
func BenchmarkResolveHeader(b *testing.B) {
	tn := newBenchHeader(b)
	ctx := b.Context()
	meta := map[string]string{"X-Tenant-ID": "acme"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := tn.Resolve(ctx, meta); err != nil {
			b.Fatalf("Resolve(): %v", err)
		}
	}
}

// BenchmarkResolveHeaderParallel measures header resolution under concurrent
// load; the adapter holds only read-only compiled regexp state.
func BenchmarkResolveHeaderParallel(b *testing.B) {
	tn := newBenchHeader(b)
	ctx := b.Context()
	meta := map[string]string{"X-Tenant-ID": "acme"}

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

// BenchmarkResolveSubdomain measures the fallback path: canonical key lookup
// misses, so Resolve parses Host, strips any port, lowercases, and regex-matches.
func BenchmarkResolveSubdomain(b *testing.B) {
	tn := newBenchHeader(b)
	ctx := b.Context()
	meta := map[string]string{"Host": "acme.example.com:8080"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := tn.Resolve(ctx, meta); err != nil {
			b.Fatalf("Resolve(): %v", err)
		}
	}
}

// BenchmarkScoped measures deriving a tenant-scoped context.
func BenchmarkScoped(b *testing.B) {
	tn := newBenchHeader(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := tn.Scoped(ctx, "acme"); err != nil {
			b.Fatalf("Scoped(): %v", err)
		}
	}
}
