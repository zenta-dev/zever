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

	for b.Loop() {
		if err := c.Set(ctx, "bench", val, 0); err != nil {
			b.Fatal(err)
		}

		if _, err := c.Get(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDelete measures a Redis-backed cache Set followed by Delete
// against an in-process miniredis server.
func BenchmarkDelete(b *testing.B) {
	c, _ := newLiveAdapter(b)
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

// BenchmarkIncrement measures the Redis INCR counter round-trip against an
// in-process miniredis server.
func BenchmarkIncrement(b *testing.B) {
	c, _ := newLiveAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if err := c.Increment(ctx, "counter"); err != nil {
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

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.Get(ctx, "bench"); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
