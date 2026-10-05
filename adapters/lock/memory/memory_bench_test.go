package memory

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

// benchLocker opens an in-process locker with a long default TTL so leases
// never lapse mid-benchmark.
func benchLocker(b *testing.B) lock.Locker {
	b.Helper()

	l, err := New(lock.Options{TTL: time.Hour})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = l.Close(b.Context()) })

	return l
}

// benchKeySeq numbers benchmark keys so each iteration uses a fresh lease.
var benchKeySeq atomic.Int64

// BenchmarkNew measures constructing and closing the locker.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		l, err := New(lock.Options{})
		if err != nil {
			b.Fatalf("New failed: %v", err)
		}

		if err := l.Close(b.Context()); err != nil {
			b.Fatalf("Close failed: %v", err)
		}
	}
}

// BenchmarkTryAcquireUnlock measures the acquire-then-release cycle on a
// fresh key.
func BenchmarkTryAcquireUnlock(b *testing.B) {
	l := benchLocker(b)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		held, ok, err := l.TryAcquire(ctx, key, time.Minute)
		if err != nil || !ok {
			b.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
		}

		if err := held.Unlock(ctx); err != nil {
			b.Fatalf("Unlock failed: %v", err)
		}
	}
}

// BenchmarkTryAcquireContended measures the rejected claim on a key already
// held by someone else.
func BenchmarkTryAcquireContended(b *testing.B) {
	l := benchLocker(b)

	ctx := b.Context()

	held, ok, err := l.TryAcquire(ctx, "hot", time.Hour)
	if err != nil || !ok {
		b.Fatalf("seed TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	b.Cleanup(func() { _ = held.Unlock(b.Context()) })

	b.ReportAllocs()

	for b.Loop() {
		_, acquired, err := l.TryAcquire(ctx, "hot", time.Minute)
		if err != nil {
			b.Fatalf("TryAcquire failed: %v", err)
		}

		if acquired {
			b.Fatal("TryAcquire acquired a held key")
		}
	}
}

// BenchmarkExtend measures renewing a held lease.
func BenchmarkExtend(b *testing.B) {
	l := benchLocker(b)

	ctx := b.Context()

	held, ok, err := l.TryAcquire(ctx, "bench-extend", time.Hour)
	if err != nil || !ok {
		b.Fatalf("seed TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	b.Cleanup(func() { _ = held.Unlock(b.Context()) })

	b.ReportAllocs()

	for b.Loop() {
		if err := held.Extend(ctx, time.Hour); err != nil {
			b.Fatalf("Extend failed: %v", err)
		}
	}
}

// BenchmarkAcquireImmediate measures the uncontended Acquire path, which
// succeeds on the first attempt.
func BenchmarkAcquireImmediate(b *testing.B) {
	l := benchLocker(b)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		held, err := l.Acquire(ctx, key, time.Minute)
		if err != nil {
			b.Fatalf("Acquire failed: %v", err)
		}

		if err := held.Unlock(ctx); err != nil {
			b.Fatalf("Unlock failed: %v", err)
		}
	}
}

// BenchmarkNewHolderID measures random holder-id generation.
func BenchmarkNewHolderID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := newHolderID(); err != nil {
			b.Fatalf("newHolderID failed: %v", err)
		}
	}
}

// BenchmarkTryAcquireParallel measures concurrent claims on distinct keys.
func BenchmarkTryAcquireParallel(b *testing.B) {
	l := benchLocker(b)

	ctx := b.Context()

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

			held, ok, err := l.TryAcquire(ctx, key, time.Minute)
			if err != nil || !ok {
				b.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
			}

			if err := held.Unlock(ctx); err != nil {
				b.Fatalf("Unlock failed: %v", err)
			}
		}
	})
}
