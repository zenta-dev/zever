package tenant

import (
	"testing"
)

// benchTenantAdapter registers a stub tenant once and returns its adapter so
// Open can be measured without the one-shot registration cost.

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
