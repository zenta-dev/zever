package redis

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/lock"
)

// benchKeySeq keeps keys unique so each iteration measures a fresh
// acquire/release rather than contention on one key.
var benchKeySeq atomic.Int64

// benchLocker opens a locker over a throwaway miniredis instance.
func benchLocker(b *testing.B) lock.Locker {
	b.Helper()

	s, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis Run: %v", err)
	}

	b.Cleanup(s.Close)

	l, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = l.Close(b.Context()) })

	return l
}

// BenchmarkTryAcquireUnlock measures the SET NX plus Lua release round trip.
func BenchmarkTryAcquireUnlock(b *testing.B) {
	l := benchLocker(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		h, ok, err := l.TryAcquire(ctx, key, 0)
		if err != nil {
			b.Fatalf("TryAcquire: %v", err)
		}
		if !ok {
			b.Fatalf("TryAcquire: not acquired")
		}
		if err := h.Unlock(ctx); err != nil {
			b.Fatalf("Unlock: %v", err)
		}
	}
}

// BenchmarkTryAcquireUnlockParallel measures concurrent acquire/release
// cycles on distinct keys over a shared pool.
func BenchmarkTryAcquireUnlockParallel(b *testing.B) {
	l := benchLocker(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

			h, ok, err := l.TryAcquire(ctx, key, 0)
			if err != nil {
				b.Fatalf("TryAcquire: %v", err)
			}
			if !ok {
				b.Fatalf("TryAcquire: not acquired")
			}
			if err := h.Unlock(ctx); err != nil {
				b.Fatalf("Unlock: %v", err)
			}
		}
	})
}
