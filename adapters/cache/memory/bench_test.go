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

	for b.Loop() {
		if err := c.Set(ctx, "bench", val, 0); err != nil {
			b.Fatal(err)
		}

		if _, err := c.Get(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDelete measures a Set followed by Delete on the same key.
func BenchmarkDelete(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()
	val := []byte("bench-value")

	b.ReportAllocs()

	for b.Loop() {
		if err := c.Set(ctx, "bench", val, 0); err != nil {
			b.Fatal(err)
		}

		if err := c.Delete(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIncrement measures the atomic read-modify-write counter path.
func BenchmarkIncrement(b *testing.B) {
	c := stubCache(b, cache.Options{})
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
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
