package tenant

import (
	"testing"
)

// benchTenantAdapter registers a stub tenant once and returns its adapter so
// Open can be measured without the one-shot registration cost.
func benchTenantAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Tenant, error) { return &stubTenant{id: "acme"}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchTenantAdapter(b)
	opts := Options{Header: "X-Tenant-ID"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchTenantAdapter(b)
	opts := Options{Header: "X-Tenant-ID"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Header: "X-Tenant-ID", SubdomainRegex: `^[a-z0-9-]+$`}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkContextRoundtrip(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		got, ok := FromContext(ContextWithTenant(ctx, "acme"))
		if !ok || got != "acme" {
			b.Fatal("context roundtrip failed")
		}
	}
}

func BenchmarkValidHeaderKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !validHeaderKey("X-Tenant-ID") {
			b.Fatal("validHeaderKey(X-Tenant-ID) = false")
		}
	}
}
