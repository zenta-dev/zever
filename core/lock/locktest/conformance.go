// Package locktest provides the conformance kit third-party lock adapters run to prove backend parity.
package locktest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

const (
	// DefaultLeaseTTL is the short TTL conformance expiry tests set before polling for release.
	DefaultLeaseTTL = 30 * time.Millisecond
	// DefaultWaitTimeout bounds how long expiry/acquire polls wait before failing.
	DefaultWaitTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between expiry-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

// Conformance verifies factory-built lockers implement the lock.Locker
// contract: open/register round-trip, exclusive TryAcquire, blocking
// Acquire with context cancellation, Extend/Unlock lease semantics
// (ErrNotHeld, no steal), TTL expiry, and Close. Each subtest takes a
// fresh instance from factory so cases stay isolated. Expiry waits poll
// with a context deadline; they never synchronize with time.Sleep and
// never touch the network.
//
// Virtual-time fakes: when the factory product also implements
// FastForward(time.Duration) (e.g. a miniredis wrapper whose TTLs advance
// only via FastForward), expiry polls advance that clock by the poll
// interval after each unsuccessful attempt so TTLs expire without
// wall-clock waiting. Real-time adapters do not implement it and are
// unaffected.
//
// No-op adapters: none; every adapter must enforce mutual exclusion.
// A stub that always reports ok=true would trivially satisfy the shape
// but violate exclusivity, so no stub exemption exists.
func Conformance(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("TryAcquireExclusive", func(t *testing.T) { conformanceExclusive(t, factory) })
	t.Run("Acquire", func(t *testing.T) { conformanceAcquire(t, factory) })
	t.Run("ExtendUnlock", func(t *testing.T) { conformanceExtendUnlock(t, factory) })
	t.Run("Expiry", func(t *testing.T) { conformanceExpiry(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := lock.Open(lock.Adapter("conformance-missing-adapter"), lock.Options{}); !errors.Is(err, lock.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := lock.Adapter("conformance-probe-lock")

	if err := lock.Register(probe, nil); !errors.Is(err, lock.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(lock.Options) (lock.Locker, error) {
		return nil, errors.New("locktest: probe factory must not run")
	}

	_ = lock.Register(probe, stub)

	if err := lock.Register(probe, stub); !errors.Is(err, lock.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceExclusive(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	h, ok, err := l.TryAcquire(ctx, "exclusive", 0)
	if err != nil || !ok || h == nil {
		t.Fatalf("TryAcquire() = %v,%v,%v want lock,true,nil", h, ok, err)
	}

	if h.Key() != "exclusive" {
		t.Errorf("Key() = %q, want exclusive", h.Key())
	}

	if _, held, heldErr := l.TryAcquire(ctx, "exclusive", 0); heldErr != nil || held {
		t.Errorf("TryAcquire(held) = ok=%v,err=%v want ok=false,nil", held, heldErr)
	}

	other, ok, err := l.TryAcquire(ctx, "other", 0)
	if err != nil || !ok || other == nil {
		t.Fatalf("TryAcquire(other) = %v,%v,%v want lock,true,nil", other, ok, err)
	}

	if _, _, err := l.TryAcquire(ctx, "", 0); err == nil {
		t.Error("TryAcquire(empty) = nil, want error")
	}

	if err := h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	if err := other.Unlock(ctx); err != nil {
		t.Fatalf("Unlock(other) error = %v", err)
	}
}

func conformanceAcquire(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	holder, ok, err := l.TryAcquire(ctx, "blocked", 0)
	if err != nil || !ok {
		t.Fatalf("TryAcquire() = ok=%v,err=%v want true,nil", ok, err)
	}

	acquired := make(chan lock.Lock, 1)
	errCh := make(chan error, 1)

	go func() {
		got, acquireErr := l.Acquire(context.Background(), "blocked", 0)
		if acquireErr != nil {
			errCh <- acquireErr
			return
		}
		acquired <- got
	}()

	select {
	case h := <-acquired:
		t.Fatalf("Acquire() returned %v while key held", h.Key())
	case waitErr := <-errCh:
		t.Fatalf("Acquire() error = %v while waiting", waitErr)
	case <-pollTick(ctx, DefaultPollInterval):
	}

	if err = holder.Unlock(ctx); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	select {
	case h := <-acquired:
		if h.Key() != "blocked" {
			t.Errorf("Key() = %q, want blocked", h.Key())
		}
		if err = h.Unlock(ctx); err != nil {
			t.Errorf("Unlock() error = %v", err)
		}
	case releaseErr := <-errCh:
		t.Fatalf("Acquire() error = %v after release", releaseErr)
	case <-ctx.Done():
		t.Fatal("Acquire() did not return after release")
	}

	holder2, ok, err := l.TryAcquire(ctx, "cancel", 0)
	if err != nil || !ok {
		t.Fatalf("TryAcquire() = ok=%v,err=%v want true,nil", ok, err)
	}
	defer func() { _ = holder2.Unlock(ctx) }()

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := l.Acquire(canceled, "cancel", 0); err == nil {
		t.Error("Acquire(canceled) = nil, want context error")
	} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Acquire(canceled) err = %v, want context cancellation", err)
	}
}

func conformanceExtendUnlock(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	h, ok, err := l.TryAcquire(ctx, "lease", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire() = ok=%v,err=%v want true,nil", ok, err)
	}

	if err = h.Extend(ctx, time.Minute); err != nil {
		t.Fatalf("Extend() error = %v", err)
	}

	if err = h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	if err = h.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("Unlock(again) err = %v, want ErrNotHeld", err)
	}

	if err = h.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("Extend(lost) err = %v, want ErrNotHeld", err)
	}

	// Expiry hands the key to a new holder; the old handle must not
	// steal it back via Unlock.
	old, ok, err := l.TryAcquire(ctx, "steal", DefaultLeaseTTL)
	if err != nil || !ok {
		t.Fatalf("TryAcquire() = ok=%v,err=%v want true,nil", ok, err)
	}

	eventually(t, "expired lease re-acquired", func(ctx context.Context) bool {
		_, stolen, stolenErr := l.TryAcquire(ctx, "steal", time.Minute)
		if stolenErr == nil && stolen {
			return true
		}

		maybeFastForward(l)

		return false
	})

	if err = old.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("Unlock(expired) err = %v, want ErrNotHeld (no steal)", err)
	}
}

func conformanceExpiry(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	if _, ok, err := l.TryAcquire(ctx, "ttl", DefaultLeaseTTL); err != nil || !ok {
		t.Fatalf("TryAcquire() = ok=%v,err=%v want true,nil", ok, err)
	}

	if _, ok, err := l.TryAcquire(ctx, "ttl", 0); err != nil || ok {
		t.Fatalf("TryAcquire(held) = ok=%v,err=%v want false,nil", ok, err)
	}

	eventually(t, "expired key re-acquirable", func(ctx context.Context) bool {
		_, ok, err := l.TryAcquire(ctx, "ttl", time.Minute)
		if err == nil && ok {
			return true
		}

		maybeFastForward(l)

		return false
	})
}

func conformanceClose(t *testing.T, factory func(t *testing.T) lock.Locker) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	if err := l.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := l.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}

// fastForwarder is implemented by conformance factories running against
// virtual-time fakes (e.g. a miniredis wrapper whose TTLs advance only via
// FastForward). Expiry polls advance it after each unsuccessful attempt.
type fastForwarder interface {
	FastForward(time.Duration)
}

// maybeFastForward advances l's virtual clock when it implements
// fastForwarder; it is a no-op for real-time adapters.
func maybeFastForward(l lock.Locker) {
	if f, ok := any(l).(fastForwarder); ok {
		f.FastForward(DefaultPollInterval)
	}
}

// eventually polls cond until true or DefaultWaitTimeout elapses. Poll
// ticks use a ticker, never time.Sleep.
func eventually(t *testing.T, msg string, cond func(ctx context.Context) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultWaitTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met within %v: %s", DefaultWaitTimeout, msg)
		case <-ticker.C:
		}
	}
}

// pollTick returns a channel that fires once after d. It backs the
// negative Acquire check without time.Sleep synchronization.
func pollTick(ctx context.Context, d time.Duration) <-chan struct{} {
	ch := make(chan struct{}, 1)

	go func() {
		t := time.NewTimer(d)
		defer t.Stop()

		select {
		case <-ctx.Done():
		case <-t.C:
			ch <- struct{}{}
		}
	}()

	return ch
}
