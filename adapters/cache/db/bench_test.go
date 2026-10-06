package db

import (
	"testing"
)

// BenchmarkSetGet measures a DB-backed cache Set followed by Get.
func BenchmarkSetGet(b *testing.B) {
	c := mustNew(b)
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

// BenchmarkDelete measures a DB-backed cache Set followed by Delete.
func BenchmarkDelete(b *testing.B) {
	c := mustNew(b)
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

// BenchmarkGet measures a warm DB-backed cache read.
func BenchmarkGet(b *testing.B) {
	c := mustNew(b)
	ctx := b.Context()
	val := []byte("bench-value")

	if err := c.Set(ctx, "bench", val, 0); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := c.Get(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetParallel measures warm-read throughput under concurrent load.
func BenchmarkGetParallel(b *testing.B) {
	c := mustNew(b)
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
