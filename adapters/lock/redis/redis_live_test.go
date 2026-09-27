package redis

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/lock"
)

// newLiveLocker starts miniredis and opens a locker against it. Each test
// gets a fresh server (distinct addr), so parallel live tests never share
// the zredis singleton client.
func newLiveLocker(t *testing.T, opts lock.Options) lock.Locker {
	t.Helper()

	s := miniredis.RunT(t)
	opts.Addr = s.Addr()

	l, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = l.Close(t.Context()) })

	return l
}

func TestNew_defaultsAndPrefix(t *testing.T) {
	s := miniredis.RunT(t)

	l, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = l.Close(t.Context()) }()

	a, ok := l.(*adapter)
	if !ok {
		t.Fatalf("New() = %T, want *adapter", l)
	}
	if a.prefix != "lock:" || a.ttl != lock.DefaultTTL || a.retryInterval != lock.DefaultRetryInterval {
		t.Errorf("defaults = %q/%v/%v, want lock:/%v/%v", a.prefix, a.ttl, a.retryInterval, lock.DefaultTTL, lock.DefaultRetryInterval)
	}

	if _, acqOK, acqErr := l.TryAcquire(t.Context(), "k", 0); acqErr != nil || !acqOK {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", acqOK, acqErr)
	}

	if got := s.TTL("lock:k"); got < 29*time.Second {
		t.Errorf("TTL(lock:k) = %v, want ~%v (adapter default)", got, lock.DefaultTTL)
	}

	custom, err := New(lock.Options{Addr: s.Addr(), Prefix: "app:", TTL: time.Minute, RetryInterval: time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = custom.Close(t.Context()) }()

	if _, ok, err := custom.TryAcquire(t.Context(), "k", 0); err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", ok, err)
	}

	if !s.Exists("app:k") {
		t.Error("custom prefix key app:k missing")
	}
}

func TestNew_badURL(t *testing.T) {
	if _, err := New(lock.Options{URL: "redis://"}); err == nil {
		t.Error("New(bad URL) succeeded, want address error")
	}
}

func TestLive_acquireUnlock(t *testing.T) {
	l := newLiveLocker(t, lock.Options{})
	ctx := t.Context()

	h, ok, err := l.TryAcquire(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", ok, err)
	}

	if h.Key() != "job" {
		t.Errorf("Key() = %q, want job", h.Key())
	}

	if err := h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	if err := h.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("second Unlock = %v, want ErrNotHeld", err)
	}
}

func TestLive_contention(t *testing.T) {
	// One server, two lockers: the zredis singleton shares the client for
	// identical addresses, so separate servers would disconnect each other.
	s := miniredis.RunT(t)

	newLocker := func(opts lock.Options) lock.Locker {
		t.Helper()

		opts.Addr = s.Addr()

		l, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return l
	}

	a := newLocker(lock.Options{})
	b := newLocker(lock.Options{})
	ctx := t.Context()

	ha, ok, err := a.TryAcquire(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("A TryAcquire = %v, %v, want true, nil", ok, err)
	}
	defer func() { _ = ha.Unlock(ctx) }()

	if _, contOK, contErr := b.TryAcquire(ctx, "job", time.Minute); contErr != nil || contOK {
		t.Errorf("B TryAcquire = %v, %v, want false, nil", contOK, contErr)
	}

	if unlockErr := ha.Unlock(ctx); unlockErr != nil {
		t.Fatalf("A Unlock: %v", unlockErr)
	}

	hb, ok, err := b.TryAcquire(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("B TryAcquire after release = %v, %v, want true, nil", ok, err)
	}

	// A's stale handle must not steal B's lease.
	if err := ha.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("stale A Unlock = %v, want ErrNotHeld", err)
	}

	_ = hb.Unlock(ctx)
}

func TestLive_acquireWaitsForExpiry(t *testing.T) {
	s := miniredis.RunT(t)

	newLocker := func() lock.Locker {
		t.Helper()

		l, err := New(lock.Options{Addr: s.Addr(), RetryInterval: 5 * time.Millisecond})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return l
	}

	a := newLocker()
	b := newLocker()
	ctx := t.Context()

	if _, ok, err := a.TryAcquire(ctx, "job", 200*time.Millisecond); err != nil || !ok {
		t.Fatalf("A TryAcquire = %v, %v, want true, nil", ok, err)
	}

	// B blocks while A holds the lease.
	done := make(chan lock.Lock, 1)
	go func() {
		h, err := b.Acquire(ctx, "job", time.Minute)
		if err != nil {
			return
		}

		done <- h
	}()

	select {
	case <-done:
		t.Fatal("B Acquire succeeded while A still holds the lease")
	case <-time.After(100 * time.Millisecond):
	}

	// miniredis expires only on FastForward, never in real time: jump past
	// A's TTL and B's next poll must succeed.
	s.FastForward(time.Second)

	select {
	case h := <-done:
		_ = h.Unlock(ctx)
	case <-time.After(5 * time.Second):
		t.Fatal("B Acquire did not succeed after A lease expired")
	}
}

func TestLive_extend(t *testing.T) {
	s := miniredis.RunT(t)
	l, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = l.Close(t.Context()) }()

	ctx := t.Context()

	h, ok, err := l.TryAcquire(ctx, "job", 500*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", ok, err)
	}

	s.FastForward(400 * time.Millisecond)

	if extendErr := h.Extend(ctx, 0); extendErr != nil {
		t.Fatalf("Extend(default): %v", extendErr)
	}

	// Without the renewal the lease would have lapsed by now.
	s.FastForward(400 * time.Millisecond)

	other, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = other.Close(t.Context()) }()

	// Same address reuses the shared client; the rival sees the same key.
	if _, ok, err := other.TryAcquire(ctx, "job", time.Minute); err != nil || ok {
		t.Errorf("rival TryAcquire = %v, %v, want false, nil", ok, err)
	}

	if err := h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestLive_extendAfterExpiryNotHeld(t *testing.T) {
	s := miniredis.RunT(t)
	l, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = l.Close(t.Context()) }()

	ctx := t.Context()

	h, ok, err := l.TryAcquire(ctx, "job", 100*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", ok, err)
	}

	s.FastForward(time.Second)

	if err := h.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("Extend after expiry = %v, want ErrNotHeld", err)
	}

	if err := h.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("Unlock after expiry = %v, want ErrNotHeld", err)
	}
}

func TestLive_prefixIsolation(t *testing.T) {
	s := miniredis.RunT(t)
	ctx := t.Context()

	a, err := New(lock.Options{Addr: s.Addr(), Prefix: "a:"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close(t.Context()) }()

	b, err := New(lock.Options{Addr: s.Addr(), Prefix: "b:"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = b.Close(t.Context()) }()

	ha, ok, err := a.TryAcquire(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("A TryAcquire = %v, %v, want true, nil", ok, err)
	}
	defer func() { _ = ha.Unlock(ctx) }()

	if _, ok, err := b.TryAcquire(ctx, "job", time.Minute); err != nil || !ok {
		t.Errorf("B TryAcquire (other prefix) = %v, %v, want true, nil", ok, err)
	}
}
