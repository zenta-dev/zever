package lock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/lock/memory"
	"github.com/zenta-dev/zever/core/lock"
)

func openMemoryLocker(t *testing.T, opts lock.Options) lock.Locker {
	t.Helper()

	a := freshLockAdapter()
	if err := lock.Register(a, memory.New); err != nil {
		t.Fatalf("Register(memory) err = %v", err)
	}

	l, err := lock.Open(a, opts)
	if err != nil {
		t.Fatalf("Open(memory) err = %v", err)
	}

	t.Cleanup(func() { _ = l.Close(context.Background()) })

	return l
}

func TestLockTryAcquire_emptyKey_returnsError(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{})

	held, ok, err := l.TryAcquire(t.Context(), "", time.Minute)
	if err == nil {
		t.Fatal("TryAcquire(empty key) err = nil, want error")
	}
	if ok {
		t.Fatal("TryAcquire(empty key) ok = true, want false")
	}
	if held != nil {
		t.Fatal("TryAcquire(empty key) returned non-nil lock")
	}
}

func TestLockTryAcquire_held_secondMisses(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{})
	ctx := t.Context()

	first, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}
	defer func() { _ = first.Unlock(ctx) }()

	second, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("second TryAcquire err = %v", err)
	}
	if ok || second != nil {
		t.Fatalf("second TryAcquire = (%v, %v), want (nil, false)", second, ok)
	}
}

func TestLockUnlock_thenReacquire(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{})
	ctx := t.Context()

	held, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}
	if unlockErr := held.Unlock(ctx); unlockErr != nil {
		t.Fatalf("Unlock err = %v", unlockErr)
	}

	again, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("re-TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}
	if err := again.Unlock(ctx); err != nil {
		t.Fatalf("re-Unlock err = %v", err)
	}
}

func TestLockExtend_afterUnlock_returnsErrNotHeld(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{})
	ctx := t.Context()

	held, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}
	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("Unlock err = %v", err)
	}
	if err := held.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Extend after Unlock err = %v, want ErrNotHeld", err)
	}
}

func TestLockClose_idempotent_andReusable(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{})
	ctx := t.Context()

	if err := l.Close(ctx); err != nil {
		t.Fatalf("first Close err = %v", err)
	}
	if err := l.Close(ctx); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}

	held, ok, err := l.TryAcquire(ctx, "after-close", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire after Close = (%v, %v), want (true, nil)", ok, err)
	}
	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("Unlock after Close err = %v", err)
	}
}

func TestLockAcquire_cancelledContext_returnsError(t *testing.T) {
	t.Parallel()

	l := openMemoryLocker(t, lock.Options{RetryInterval: time.Hour})

	held, ok, err := l.TryAcquire(t.Context(), "held", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}
	defer func() { _ = held.Unlock(context.Background()) }()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := l.Acquire(ctx, "held", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire cancelled err = %v, want context.Canceled", err)
	}
}
