package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/lock"
)

func newLocker(t *testing.T, o lock.Options) lock.Locker {
	t.Helper()

	l, err := New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = l.Close(t.Context()) })

	return l
}

func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	const timeout = 2 * time.Second

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestNew_appliesOptions(t *testing.T) {
	t.Parallel()

	l, err := New(lock.Options{
		Prefix:        "p:",
		TTL:           2 * time.Minute,
		RetryInterval: 7 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("locker type = %T, want *adapter", l)
	}

	if a.prefix != "p:" || a.ttl != 2*time.Minute || a.retryInterval != 7*time.Millisecond {
		t.Fatalf("adapter = %+v", a)
	}
}

func TestNew_defaults(t *testing.T) {
	t.Parallel()

	l, err := New(lock.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("locker type = %T, want *adapter", l)
	}

	if a.ttl != lock.DefaultTTL || a.retryInterval != lock.DefaultRetryInterval {
		t.Fatalf("adapter = %+v, want framework defaults", a)
	}

	if a.leases == nil {
		t.Fatal("leases map is nil")
	}
}

func TestTryAcquire_excludesSecondHolder(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	first, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire = (%v, %v), want (true, nil)", ok, err)
	}

	if first.Key() != "k" {
		t.Fatalf("key = %q, want k", first.Key())
	}

	second, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}

	if ok || second != nil {
		t.Fatal("second acquire succeeded while lock was held")
	}

	if err := first.Unlock(ctx); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	if _, ok, err := l.TryAcquire(ctx, "k", time.Minute); err != nil || !ok {
		t.Fatalf("acquire after unlock = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestTryAcquire_distinctKeysDoNotBlock(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	for _, k := range []string{"a", "b", "c"} {
		if _, ok, err := l.TryAcquire(ctx, k, time.Minute); err != nil || !ok {
			t.Fatalf("acquire %q = (%v, %v), want (true, nil)", k, ok, err)
		}
	}
}

func TestTryAcquire_emptyKeyRejected(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})

	if _, _, err := l.TryAcquire(t.Context(), "", time.Minute); err == nil {
		t.Fatal("empty key: want error, got nil")
	}
}

func TestTryAcquire_nonPositiveTTLFallsBackToDefault(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, ok, err := l.TryAcquire(ctx, "k", 0)
	if err != nil || !ok {
		t.Fatalf("acquire = (%v, %v), want (true, nil)", ok, err)
	}

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("locker = %T, want *adapter", l)
	}
	a.mu.Lock()
	ttl := time.Until(a.leases[a.prefix+"k"].expires)
	a.mu.Unlock()

	if ttl < 29*time.Second {
		t.Fatalf("lease ttl = %v, want default %v", ttl, lock.DefaultTTL)
	}

	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

func TestTryAcquire_holderIDFailureReturnsError(t *testing.T) {
	// Not parallel: stubs the process-wide randRead seam.
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom") }
	defer func() { randRead = old }()

	l := newLocker(t, lock.Options{})

	if _, _, err := l.TryAcquire(t.Context(), "k", time.Minute); err == nil {
		t.Fatal("TryAcquire with failing rand: want error, got nil")
	}
}

func TestLease_autoExpires(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	if _, ok, err := l.TryAcquire(ctx, "k", 20*time.Millisecond); err != nil || !ok {
		t.Fatalf("acquire = (%v, %v), want (true, nil)", ok, err)
	}

	// Poll re-acquire until the 20ms lease lapses; a live lease reports
	// ok=false, an expired one re-acquires. Unlock the probe so the final
	// assertion starts unheld.
	eventually(t, func() bool {
		probe, ok, err := l.TryAcquire(ctx, "k", time.Minute)
		if err != nil || !ok {
			return false
		}
		_ = probe.Unlock(ctx)
		return true
	}, "lease expiry")

	if _, ok, err := l.TryAcquire(ctx, "k", time.Minute); err != nil || !ok {
		t.Fatalf("acquire after expiry = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestExtend_keepsLockHeld(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, ok, err := l.TryAcquire(ctx, "k", 40*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire = (%v, %v), want (true, nil)", ok, err)
	}

	// Extend immediately, well before the 40ms lease lapses.
	if err := held.Extend(ctx, time.Minute); err != nil {
		t.Fatalf("extend: %v", err)
	}

	// Wait past the original 40ms TTL, polling that the lock stays held.
	start := time.Now()
	eventually(t, func() bool {
		if time.Since(start) <= 60*time.Millisecond {
			return false
		}
		_, ok, _ := l.TryAcquire(ctx, "k", time.Minute)
		return !ok
	}, "extended lease to stay held")

	if _, ok, err := l.TryAcquire(ctx, "k", time.Minute); err != nil || ok {
		t.Fatalf("acquire after extend = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestExtend_afterExpiryFails(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, _, err := l.TryAcquire(ctx, "k", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Poll takeover until the 20ms lease lapses (live lease reports
	// ok=false without side effects), then release the probe so the
	// stale Extend observes a free key.
	eventually(t, func() bool {
		probe, ok, err := l.TryAcquire(ctx, "k", time.Minute)
		if err != nil || !ok {
			return false
		}
		_ = probe.Unlock(ctx)
		return true
	}, "lease expiry")

	if err := held.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("extend after expiry = %v, want ErrNotHeld", err)
	}
}

func TestExtend_nonPositiveTTLFallsBackToDefault(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, _, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := held.Extend(ctx, 0); err != nil {
		t.Fatalf("extend with default ttl: %v", err)
	}
}

func TestExtend_afterUnlockFails(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, _, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	if err := held.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("extend after unlock = %v, want ErrNotHeld", err)
	}
}

func TestUnlock_afterTakeoverDoesNotStealLock(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	stale, _, err := l.TryAcquire(ctx, "k", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Poll takeover until the 20ms lease lapses.
	var newHolder lock.Lock
	eventually(t, func() bool {
		got, ok, err := l.TryAcquire(ctx, "k", time.Minute)
		if err != nil || !ok {
			return false
		}
		newHolder = got
		return true
	}, "takeover acquire")

	if newHolder == nil {
		t.Fatal("takeover acquire never succeeded")
	}

	if err := stale.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("stale unlock = %v, want ErrNotHeld", err)
	}

	if _, ok, _ := l.TryAcquire(ctx, "k", time.Minute); ok {
		t.Fatal("stale unlock released the new holder's lock")
	}

	if err := newHolder.Unlock(ctx); err != nil {
		t.Fatalf("new holder unlock: %v", err)
	}
}

func TestUnlock_doubleUnlockReportsNotHeld(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	held, _, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	if err := held.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("second unlock = %v, want ErrNotHeld", err)
	}
}

func TestAcquire_blocksUntilReleased(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{RetryInterval: 5 * time.Millisecond})

	held, _, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	acquired := make(chan lock.Lock, 1)

	go func() {
		got, err := l.Acquire(ctx, "k", time.Minute)
		if err != nil {
			close(acquired)
			return
		}

		acquired <- got
	}()

	select {
	case <-acquired:
		t.Fatal("Acquire returned while the lock was still held")
	case <-time.After(30 * time.Millisecond):
	}

	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	select {
	case got, ok := <-acquired:
		if !ok || got == nil {
			t.Fatal("Acquire failed after the lock was released")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not return after the lock was released")
	}
}

func TestAcquire_honoursContextCancellation(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{RetryInterval: 5 * time.Millisecond})

	if _, _, err := l.TryAcquire(t.Context(), "k", time.Minute); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()

	got, err := l.Acquire(ctx, "k", time.Minute)
	if err == nil {
		t.Fatal("Acquire: want error on cancelled context, got nil")
	}

	if got != nil {
		t.Fatal("Acquire returned a lock alongside its error")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestAcquire_emptyKeyReturnsError(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{RetryInterval: time.Millisecond})

	if _, err := l.Acquire(t.Context(), "", time.Minute); err == nil {
		t.Fatal("Acquire with empty key: want error, got nil")
	}
}

func TestClose_clearsLeases(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	if _, _, err := l.TryAcquire(ctx, "k", time.Minute); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := l.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("locker = %T, want *adapter", l)
	}
	a.mu.Lock()
	n := len(a.leases)
	a.mu.Unlock()

	if n != 0 {
		t.Fatalf("leases after close = %d, want 0", n)
	}
}

func TestConcurrentTryAcquire_singleWinner(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{})

	const goroutines = 64

	var (
		winners atomic.Int64
		failed  atomic.Int64
		start   = make(chan struct{})
		wg      sync.WaitGroup
	)

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			<-start

			held, ok, err := l.TryAcquire(ctx, "hot", time.Minute)
			if err != nil {
				failed.Add(1)
				return
			}

			if ok && held != nil {
				winners.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := winners.Load(); got != 1 {
		t.Fatalf("winners = %d, want exactly 1", got)
	}

	if got := failed.Load(); got != 0 {
		t.Fatalf("errors = %d, want 0", got)
	}
}

func TestConcurrentAcquire_serialisesCriticalSection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := newLocker(t, lock.Options{RetryInterval: time.Millisecond})

	const goroutines = 32

	var (
		counter  int
		inside   atomic.Int64
		overlaps atomic.Int64
		errs     atomic.Int64
		wg       sync.WaitGroup
	)

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			held, err := l.Acquire(ctx, "critical", time.Minute)
			if err != nil {
				errs.Add(1)
				return
			}

			if inside.Add(1) != 1 {
				overlaps.Add(1)
			}

			counter++

			inside.Add(-1)

			if err := held.Unlock(ctx); err != nil {
				errs.Add(1)
			}
		}()
	}

	wg.Wait()

	if got := errs.Load(); got != 0 {
		t.Fatalf("errors = %d, want 0", got)
	}

	if got := overlaps.Load(); got != 0 {
		t.Fatalf("overlapping critical sections = %d, want 0", got)
	}

	if counter != goroutines {
		t.Fatalf("counter = %d, want %d", counter, goroutines)
	}
}
