package memory

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/lock"
)

// benchKeySeq keeps keys unique so each iteration measures a fresh
// acquire/release rather than contention on one key.
var benchKeySeq atomic.Int64

// benchLocker opens an in-process locker with defaults.
func benchLocker(b *testing.B) lock.Locker {
	b.Helper()

	l, err := New(lock.Options{})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = l.Close(b.Context()) })

	return l
}

// BenchmarkTryAcquireUnlock measures the uncontended acquire/release cycle.
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

// BenchmarkTryAcquireUnlockParallel measures concurrent uncontended
// acquire/release cycles on distinct keys.
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
