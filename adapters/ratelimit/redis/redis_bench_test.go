package redis

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/ratelimit"
)

func newBenchLimiter(b *testing.B) ratelimit.Limiter {
	b.Helper()
	s := miniredis.RunT(b)
	l, err := New(ratelimit.Options{
		Rate:  1e9,
		Burst: 1000,
		Redis: ratelimit.RedisOptions{Addr: s.Addr()},
	})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = l.Close() })
	return l
}

// BenchmarkAllow measures the Lua-script allow round-trip against in-process
// miniredis.
func BenchmarkAllow(b *testing.B) {
	l := newBenchLimiter(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := l.Allow(ctx, "hot", 1); err != nil {
			b.Fatalf("Allow() = %v", err)
		}
	}
}

// BenchmarkAllowParallel measures concurrent allows over distinct keys.
func BenchmarkAllowParallel(b *testing.B) {
	l := newBenchLimiter(b)
	var seq atomic.Int64
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		key := "k-" + strconv.FormatInt(seq.Add(1), 10)
		for pb.Next() {
			if _, err := l.Allow(ctx, key, 1); err != nil {
				b.Errorf("Allow() = %v", err)
				return
			}
		}
	})
}
