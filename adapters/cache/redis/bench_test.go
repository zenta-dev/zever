package redis

import (
	"testing"
)

// BenchmarkSetGet measures a Redis-backed cache Set followed by Get against an
// in-process miniredis server.
func BenchmarkSetGet(b *testing.B) {
	c, _ := newLiveAdapter(b)
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

// BenchmarkGetParallel measures warm-read throughput under concurrent load
// against an in-process miniredis server.
func BenchmarkGetParallel(b *testing.B) {
	c, _ := newLiveAdapter(b)
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
