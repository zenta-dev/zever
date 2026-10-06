package lrucache

import (
	"testing"
	"time"
)

// BenchmarkCacheGet measures a cache hit, which also updates recency.
func BenchmarkCacheGet(b *testing.B) {
	c := New[int, int](1024)
	for i := range 1024 {
		c.Put(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		_, _ = c.Get(i % 1024)
	}
}

// BenchmarkCachePut measures inserting distinct keys under capacity.
func BenchmarkCachePut(b *testing.B) {
	c := New[int, int](1024)

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		c.Put(i%1024, i)
	}
}

// BenchmarkCacheGetOrCompute measures the present-key fast path.
func BenchmarkCacheGetOrCompute(b *testing.B) {
	c := New[int, int](1024)
	for i := range 1024 {
		c.Put(i, i)
	}

	compute := func() int { return -1 }

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		_, _ = c.GetOrCompute(i%1024, compute)
	}
}

// BenchmarkCacheGetParallel measures concurrent hits on a shared cache.
func BenchmarkCacheGetParallel(b *testing.B) {
	c := New[int, int](1024)
	for i := range 1024 {
		c.Put(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = c.Get(i % 1024)
			i++
		}
	})
}

// BenchmarkTTLCacheGet measures a TTLCache hit.
func BenchmarkTTLCacheGet(b *testing.B) {
	c := NewTTL[int, int](1024, time.Hour)
	for i := range 1024 {
		c.Put(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_, _ = c.Get(i % 1024)
		i++
	}
}

// BenchmarkTTLCachePut measures inserting distinct keys under capacity.
func BenchmarkTTLCachePut(b *testing.B) {
	c := NewTTL[int, int](1024, time.Hour)

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		c.Put(i%1024, i)
		i++
	}
}

// BenchmarkCacheDelete measures removing a present key.
func BenchmarkCacheDelete(b *testing.B) {
	c := New[int, int](1024)
	for i := range 1024 {
		c.Put(i, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		c.Delete(i % 1024)
		i++
	}
}

// BenchmarkCacheClear measures dropping all entries.
func BenchmarkCacheClear(b *testing.B) {
	c := New[int, int](1024)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for i := range 1024 {
			c.Put(i, i)
		}

		c.Clear(nil)
	}
}
