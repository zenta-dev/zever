package lock_test

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

// benchLock is a no-op lock.Lock so the benchmarks below measure the
// battery's interface dispatch cost, not any adapter's storage.
type benchLock struct{ key string }

func (l benchLock) Key() string                                 { return l.key }
func (l benchLock) Extend(context.Context, time.Duration) error { return nil }
func (l benchLock) Unlock(context.Context) error                { return nil }

// benchLocker is a no-op lock.Locker pairing with benchLock.
type benchLocker struct{}

func (benchLocker) TryAcquire(_ context.Context, key string, _ time.Duration) (lock.Lock, bool, error) {
	return benchLock{key: key}, true, nil
}

func (benchLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lock, error) {
	return benchLock{key: key}, nil
}

func (benchLocker) Close(context.Context) error { return nil }

var _ lock.Locker = benchLocker{}
var _ lock.Lock = benchLock{}

// BenchmarkOptionsValidate measures validating a populated Options value.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := lock.Options{Prefix: "bench", TTL: time.Minute, RetryInterval: time.Millisecond, MaxEntries: 128}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

// BenchmarkRef measures building the typed reference to a named lock
// service instance.
func BenchmarkRef(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if ref := lock.Ref("bench"); ref.Name != "bench" {
			b.Fatalf("Ref().Name = %q, want %q", ref.Name, "bench")
		}
	}
}

// BenchmarkDefaultRef measures building the typed reference to the default
// lock service instance.
func BenchmarkDefaultRef(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = lock.DefaultRef()
	}
}

// BenchmarkTryAcquireUnlock measures one acquire-then-release cycle through
// the Locker interface.
func BenchmarkTryAcquireUnlock(b *testing.B) {
	var l lock.Locker = benchLocker{}

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		held, ok, err := l.TryAcquire(ctx, "bench", time.Minute)
		if err != nil || !ok {
			b.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
		}

		if err := held.Unlock(ctx); err != nil {
			b.Fatalf("Unlock() error = %v", err)
		}
	}
}

// BenchmarkAcquireUnlock measures the blocking-acquire path when the lease
// is immediately available.
func BenchmarkAcquireUnlock(b *testing.B) {
	var l lock.Locker = benchLocker{}

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		held, err := l.Acquire(ctx, "bench", time.Minute)
		if err != nil {
			b.Fatalf("Acquire() error = %v", err)
		}

		if err := held.Unlock(ctx); err != nil {
			b.Fatalf("Unlock() error = %v", err)
		}
	}
}

// BenchmarkExtend measures renewing a held lease through the Lock
// interface.
func BenchmarkExtend(b *testing.B) {
	held, err := benchLocker{}.Acquire(b.Context(), "bench-extend", time.Hour)
	if err != nil {
		b.Fatalf("Acquire() error = %v", err)
	}

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := held.Extend(ctx, time.Hour); err != nil {
			b.Fatalf("Extend() error = %v", err)
		}
	}
}

// BenchmarkTryAcquireParallel measures concurrent claims through the Locker
// interface.
func BenchmarkTryAcquireParallel(b *testing.B) {
	var l lock.Locker = benchLocker{}

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			held, ok, err := l.TryAcquire(ctx, "bench", time.Minute)
			if err != nil || !ok {
				b.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
			}

			if err := held.Unlock(ctx); err != nil {
				b.Fatalf("Unlock() error = %v", err)
			}
		}
	})
}
