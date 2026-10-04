package lock_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/lock/memory"
	"github.com/zenta-dev/zever/core/lock"
)

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lock.Register(freshLockAdapter(), func(lock.Options) (lock.Locker, error) { return stubLocker{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshLockAdapter()
	if err := lock.Register(a, func(lock.Options) (lock.Locker, error) { return stubLocker{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := lock.Open(a, lock.Options{}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshLockAdapter()
	if err := lock.Register(a, func(lock.Options) (lock.Locker, error) { return stubLocker{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := lock.Open(a, lock.Options{}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lock.ParseAdapter("redis")
	}
}

func BenchmarkAdapterString(b *testing.B) {
	a := lock.Adapter("redis")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.String()
	}
}

func BenchmarkTryAcquire(b *testing.B) {
	a := freshLockAdapter()
	if err := lock.Register(a, memory.New); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	l, err := lock.Open(a, lock.Options{})
	if err != nil {
		b.Fatalf("Open err = %v", err)
	}
	b.Cleanup(func() { _ = l.Close(b.Context()) })

	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		held, ok, acqErr := l.TryAcquire(ctx, "bench-key", time.Minute)
		if acqErr != nil {
			b.Fatalf("TryAcquire err = %v", acqErr)
		}
		if ok {
			_ = held.Unlock(ctx)
		}
	}
}
