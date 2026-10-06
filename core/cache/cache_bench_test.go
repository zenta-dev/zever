package cache_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
)

// benchCacheAdapter registers a stub factory once and returns its adapter so
// Open can be measured without paying the one-shot registration cost.
func benchCacheAdapter(b *testing.B) cache.Adapter {
	b.Helper()

	a := freshCacheAdapter()
	if err := cache.Register(a, func(cache.Options) (cache.Cache, error) { return stubCache{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(cache.Options) (cache.Cache, error) { return stubCache{}, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a := freshCacheAdapter()
		if err := cache.Register(a, factory); err != nil {
			b.Fatalf("Register(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpen measures the registry lookup plus factory invocation hot path.
func BenchmarkOpen(b *testing.B) {
	a := benchCacheAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cache.Open(a, cache.Options{}); err != nil {
			b.Fatalf("Open(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpenParallel measures concurrent Open calls on one adapter.
func BenchmarkOpenParallel(b *testing.B) {
	a := benchCacheAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := cache.Open(a, cache.Options{}); err != nil {
				b.Errorf("Open(%v) err = %v", a, err)

				return
			}
		}
	})
}

// BenchmarkOptionsValidate measures cache options validation.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := cache.Options{Capacity: 1024}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkOpenShared measures the shared-registry lookup plus factory path.
func BenchmarkOpenShared(b *testing.B) {
	a := freshCacheAdapter()
	if err := cache.RegisterShared(a, func(coredb.DB, cache.Options) (cache.Cache, error) {
		return stubCache{}, nil
	}); err != nil {
		b.Fatalf("RegisterShared(%v) err = %v", a, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cache.OpenShared(a, nil, cache.Options{}); err != nil {
			b.Fatalf("OpenShared(%v) err = %v", a, err)
		}
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cache.ParseAdapter("memory"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
		}
	}
}
