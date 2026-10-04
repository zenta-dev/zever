package memory

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

// BenchmarkSetGet measures an in-memory Set followed by Get.
func BenchmarkSetGet(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()
	val := []byte("bench-value")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := c.Set(ctx, "bench", val, 0); err != nil {
			b.Fatal(err)
		}

		if _, err := c.Get(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIncrement measures the atomic read-modify-write counter path.
func BenchmarkIncrement(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := c.Increment(ctx, "counter"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetParallel measures warm-read throughput under concurrent load.
func BenchmarkGetParallel(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()
	val := []byte("bench-value")

	if err := c.Set(ctx, "bench", val, 0); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.Get(ctx, "bench"); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// BenchmarkSetParallel measures write throughput under concurrent load with a
// distinct key per iteration so entries do not collide.
func BenchmarkSetParallel(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()
	val := []byte("bench-value")

	var n atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := "bench-" + strconv.FormatInt(n.Add(1), 10)
			if err := c.Set(ctx, key, val, 0); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
