package cache_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

// BenchmarkOptionsValidate measures Options validation for a cache backend.
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
