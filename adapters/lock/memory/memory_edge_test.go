package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

func TestEdge_AcquireFreeKeyWithCancelledContext(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{RetryInterval: time.Millisecond})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Acquire attempts once before observing ctx, so a free key still wins.
	held, err := l.Acquire(ctx, "free", time.Minute)
	if err != nil {
		t.Fatalf("Acquire(free, cancelled) = %v, want nil", err)
	}

	if err := held.Unlock(context.Background()); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestEdge_NewHolderIDUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 64)

	for range 64 {
		id, err := newHolderID()
		if err != nil {
			t.Fatalf("newHolderID: %v", err)
		}

		if len(id) != 32 {
			t.Fatalf("holder id = %q, want 32 hex chars", id)
		}

		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate holder id %q", id)
		}

		seen[id] = struct{}{}
	}
}

func TestEdge_NewNegativeRetryIntervalUsesDefault(t *testing.T) {
	t.Parallel()

	l, err := New(lock.Options{TTL: -time.Second, RetryInterval: -time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = l.Close(t.Context()) })

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("locker type = %T, want *adapter", l)
	}

	if a.ttl != lock.DefaultTTL || a.retryInterval != lock.DefaultRetryInterval {
		t.Fatalf("defaults = %v/%v, want %v/%v", a.ttl, a.retryInterval, lock.DefaultTTL, lock.DefaultRetryInterval)
	}
}

func TestEdge_UnlockAfterCloseReportsNotHeld(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})
	ctx := t.Context()

	held, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	if err := l.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := held.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Unlock after Close = %v, want ErrNotHeld", err)
	}
}

func TestEdge_TryAcquireConcurrentDistinctKeys(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})
	ctx := t.Context()

	const goroutines = 64

	var failures atomic.Int64

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()

			held, ok, err := l.TryAcquire(ctx, fmt.Sprintf("key-%d", i), time.Minute)
			if err != nil || !ok {
				failures.Add(1)
				return
			}

			_ = held.Unlock(ctx)
		}(i)
	}

	wg.Wait()

	if got := failures.Load(); got != 0 {
		t.Fatalf("failures = %d, want 0", got)
	}
}

// TestEdge_RegisterOpensViaCore proves Register wires the adapter factory into
// the lock battery registry so lock.Open resolves it.
func TestEdge_RegisterOpensViaCore(t *testing.T) {
	Register()

	l, err := lock.Open(lock.Memory, lock.Options{})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	t.Cleanup(func() { _ = l.Close(t.Context()) })

	held, ok, err := l.TryAcquire(t.Context(), "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	if unlockErr := held.Unlock(t.Context()); unlockErr != nil {
		t.Fatalf("Unlock = %v", unlockErr)
	}
}
