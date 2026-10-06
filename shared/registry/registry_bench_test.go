package registry_test

import (
	"testing"

	"github.com/zenta-dev/zever/shared/registry"
)

// BenchmarkLookup measures a warm registry hit.
func BenchmarkLookup(b *testing.B) {
	r := newTestRegistry()
	if err := r.Register(adapterA, func() string { return "a" }); err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := r.Lookup(adapterA); err != nil {
			b.Fatalf("Lookup() error = %v", err)
		}
	}
}

// BenchmarkRegisterDuplicate measures the duplicate-guard path.
func BenchmarkRegisterDuplicate(b *testing.B) {
	r := newTestRegistry()
	factory := func() string { return "a" }

	if err := r.Register(adapterA, factory); err != nil {
		b.Fatalf("Register() setup error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = r.Register(adapterA, factory)
	}
}

// BenchmarkRegister measures the first-time registration path with a fresh
// registry per iteration, so every call exercises insertion rather than the
// duplicate guard.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := registry.New[string, func() string](errNil, nil, nil)
		_ = r.Register("adapter-a", func() string { return "a" })
	}
}

// BenchmarkLookupParallel measures concurrent warm hits.
func BenchmarkLookupParallel(b *testing.B) {
	r := newTestRegistry()
	if err := r.Register(adapterA, func() string { return "a" }); err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = r.Lookup(adapterA)
		}
	})
}
