package cache_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

// benchCacheAdapter registers a stub factory once and returns its adapter so
// Open can be measured without paying the one-shot registration cost.
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
