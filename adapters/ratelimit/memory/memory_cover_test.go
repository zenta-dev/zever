package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/ratelimit"
)

func newCoverStore(t *testing.T, opts ratelimit.Options) *store {
	t.Helper()

	l, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, ok := l.(*store)
	if !ok {
		t.Fatalf("New returned %T, want *store", l)
	}

	t.Cleanup(func() { _ = l.Close() })

	return s
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func TestCoverNewDefaults(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	if s.idle != ratelimit.DefaultIdleTTL {
		t.Errorf("idle = %v, want %v", s.idle, ratelimit.DefaultIdleTTL)
	}

	if s.sweep != ratelimit.DefaultSweepInterval {
		t.Errorf("sweep = %v, want %v", s.sweep, ratelimit.DefaultSweepInterval)
	}

	if s.rate != 10 {
		t.Errorf("rate = %v, want 10", s.rate)
	}

	if s.burst != 5 {
		t.Errorf("burst = %v, want 5", s.burst)
	}
}

func TestCoverNewExplicit(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{
		Rate:          7,
		Burst:         4,
		IdleTTL:       time.Minute,
		SweepInterval: 30 * time.Second,
	})
	if s.idle != time.Minute {
		t.Errorf("idle = %v, want 1m", s.idle)
	}

	if s.sweep != 30*time.Second {
		t.Errorf("sweep = %v, want 30s", s.sweep)
	}

	if s.rate != 7 {
		t.Errorf("rate = %v, want 7", s.rate)
	}

	if s.burst != 4 {
		t.Errorf("burst = %v, want 4", s.burst)
	}
}

func TestCoverAllowValidation(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	ctx := t.Context()

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := s.Allow(cancelCtx, "k", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Allow canceled ctx err = %v, want context.Canceled", err)
	}

	if _, err := s.Allow(ctx, "", 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
		t.Errorf("Allow bad key err = %v, want ErrInvalidKey", err)
	}

	if _, err := s.Allow(ctx, "k", 0); !errors.Is(err, ratelimit.ErrInvalidCost) {
		t.Errorf("Allow bad cost err = %v, want ErrInvalidCost", err)
	}

	l, err := New(ratelimit.Options{Rate: 10, Burst: 5})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	if _, err := l.Allow(ctx, "k", 1); !errors.Is(err, ratelimit.ErrClosed) {
		t.Errorf("Allow after close err = %v, want ErrClosed", err)
	}
}

func TestCoverAllowPostLockClosed(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})

	s.mu.Lock()

	type result struct {
		err error
	}

	done := make(chan result, 1)

	go func() {
		_, err := s.Allow(t.Context(), "k-postlock", 1)
		done <- result{err: err}
	}()

	// The test holds s.mu, so the goroutine blocks on it after the
	// pre-lock checks; closing underneath exercises the post-lock path
	// deterministically without any timing wait.
	s.closed.Store(true)
	s.mu.Unlock()

	select {
	case r := <-done:
		if !errors.Is(r.err, ratelimit.ErrClosed) {
			t.Fatalf("Allow post-lock close err = %v, want ErrClosed", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Allow did not return after unlock")
	}
}

func TestCoverAllowBucketMath(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	ctx := t.Context()

	d, err := s.Allow(ctx, "k-math", 2)
	if err != nil || !d.Allowed {
		t.Fatalf("Allow = %+v, err = %v, want allowed", d, err)
	}

	if d.Remaining != 3 {
		t.Errorf("Remaining = %v, want 3", d.Remaining)
	}

	d, err = s.Allow(ctx, "k-big", 10)
	if err != nil || !d.Allowed {
		t.Fatalf("Allow oversized = %+v, err = %v, want allowed (capped)", d, err)
	}

	if d.Remaining != 0 {
		t.Errorf("Remaining oversized = %v, want 0", d.Remaining)
	}

	d, err = s.Allow(ctx, "k-math", 4)
	if err != nil {
		t.Fatalf("Allow drain failed: %v", err)
	}

	if d.Allowed {
		t.Fatal("Allow over remaining succeeded, want denied")
	}

	if d.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0", d.RetryAfter)
	}
}

func TestCoverAllowRefillCapped(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 5, Burst: 3})
	ctx := t.Context()

	if _, err := s.Allow(ctx, "k-cap", 1); err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	// Simulate ~300ms of refill at 5/s without sleeping: backdate last so
	// the next Allow accrues and caps at burst.
	s.mu.Lock()
	s.buckets["k-cap"].last = time.Now().Add(-300 * time.Millisecond)
	s.mu.Unlock()

	d, err := s.Allow(ctx, "k-cap", 1)
	if err != nil || !d.Allowed {
		t.Fatalf("Allow post-sleep = %+v, err = %v, want allowed", d, err)
	}

	if d.Remaining > 3 {
		t.Errorf("Remaining = %v, want <= burst 3", d.Remaining)
	}

	if d.Remaining != 2 {
		t.Errorf("Remaining = %v, want 2 (capped refill minus cost)", d.Remaining)
	}
}

func TestCoverAllowIdleExpiryWhiteBox(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 1, Burst: 2, IdleTTL: time.Minute})
	ctx := t.Context()

	if _, err := s.Allow(ctx, "k-idle", 2); err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	d, err := s.Allow(ctx, "k-idle", 1)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if d.Allowed {
		t.Fatal("expected denial before idle expiry")
	}

	s.mu.Lock()
	s.buckets["k-idle"].last = time.Now().Add(-2 * s.idle)
	s.mu.Unlock()

	d, err = s.Allow(ctx, "k-idle", 2)
	if err != nil || !d.Allowed {
		t.Fatalf("post-expiry Allow = %+v, err = %v, want allowed", d, err)
	}
}

func TestCoverAllowFutureLastClamp(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	ctx := t.Context()

	d, err := s.Allow(ctx, "k-future", 1)
	if err != nil || !d.Allowed {
		t.Fatalf("Allow = %+v, err = %v, want allowed", d, err)
	}

	if d.Remaining != 4 {
		t.Fatalf("Remaining = %v, want 4", d.Remaining)
	}

	s.mu.Lock()
	s.buckets["k-future"].last = time.Now().Add(time.Second)
	s.mu.Unlock()

	d, err = s.Allow(ctx, "k-future", 1)
	if err != nil || !d.Allowed {
		t.Fatalf("Allow future-last = %+v, err = %v, want allowed", d, err)
	}

	if d.Remaining != 3 {
		t.Errorf("Remaining = %v, want 3 (negative elapsed clamped)", d.Remaining)
	}
}

func TestCoverResetPaths(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	ctx := t.Context()

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if err := s.Reset(cancelCtx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Reset canceled ctx err = %v, want context.Canceled", err)
	}

	if err := s.Reset(ctx, ""); !errors.Is(err, ratelimit.ErrInvalidKey) {
		t.Errorf("Reset bad key err = %v, want ErrInvalidKey", err)
	}

	l, err := New(ratelimit.Options{Rate: 10, Burst: 5})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	if resetErr := l.Reset(ctx, "k"); !errors.Is(resetErr, ratelimit.ErrClosed) {
		t.Errorf("Reset after close err = %v, want ErrClosed", resetErr)
	}

	if resetErr := s.Reset(ctx, "k-missing"); resetErr != nil {
		t.Errorf("Reset missing = %v, want nil", resetErr)
	}

	if _, allowErr := s.Allow(ctx, "k-del", 1); allowErr != nil {
		t.Fatalf("Allow failed: %v", allowErr)
	}

	if resetErr := s.Reset(ctx, "k-del"); resetErr != nil {
		t.Fatalf("Reset failed: %v", resetErr)
	}

	s.mu.RLock()
	_, ok := s.buckets["k-del"]
	s.mu.RUnlock()

	if ok {
		t.Fatal("bucket still present after Reset")
	}

	d, err := s.Allow(ctx, "k-del", 5)
	if err != nil || !d.Allowed {
		t.Fatalf("post-reset Allow = %+v, err = %v, want allowed", d, err)
	}
}

func TestCoverResetPostLockClosed(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})

	s.mu.Lock()

	done := make(chan error, 1)

	go func() {
		done <- s.Reset(t.Context(), "k-postlock")
	}()

	// Same as Allow post-lock: the goroutine parks on s.mu, which this
	// test holds, so close underneath without any timing wait.
	s.closed.Store(true)
	s.mu.Unlock()

	select {
	case err := <-done:
		if !errors.Is(err, ratelimit.ErrClosed) {
			t.Fatalf("Reset post-lock close err = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Reset did not return after unlock")
	}
}

func TestCoverCloseIdempotentWipes(t *testing.T) {
	l, err := New(ratelimit.Options{Rate: 10, Burst: 5})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, ok := l.(*store)
	if !ok {
		t.Fatalf("New returned %T, want *store", l)
	}

	if _, err := l.Allow(t.Context(), "k", 1); err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	s.mu.RLock()
	n := len(s.buckets)
	s.mu.RUnlock()

	if n != 0 {
		t.Errorf("buckets after Close = %d, want 0", n)
	}

	if err := l.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestCoverRunTickerSweepsAndStops(t *testing.T) {
	l, err := New(ratelimit.Options{Rate: 10, Burst: 3, IdleTTL: 40 * time.Millisecond, SweepInterval: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, ok := l.(*store)
	if !ok {
		t.Fatalf("New returned %T, want *store", l)
	}

	if _, err := l.Allow(t.Context(), "k-sweep", 1); err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	s.mu.Lock()
	s.buckets["k-sweep"].last = time.Now().Add(-time.Second)
	s.mu.Unlock()

	eventually(t, 2*time.Second, func() bool {
		s.mu.RLock()
		defer s.mu.RUnlock()
		_, ok := s.buckets["k-sweep"]
		return !ok
	}, "background sweeper to remove expired bucket")

	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
}

func TestCoverSweepOnceTable(t *testing.T) {
	now := time.Now()

	empty := &store{buckets: map[string]*bucket{}, idle: time.Minute}
	empty.sweepOnce()

	if len(empty.buckets) != 0 {
		t.Errorf("empty sweep buckets = %d, want 0", len(empty.buckets))
	}

	kept := &store{
		buckets: map[string]*bucket{"fresh": {tokens: 1, last: now}},
		idle:    time.Minute,
	}
	kept.sweepOnce()

	if _, ok := kept.buckets["fresh"]; !ok {
		t.Error("unexpired bucket removed, want kept")
	}

	mixed := &store{
		buckets: map[string]*bucket{
			"old":   {tokens: 0, last: now.Add(-2 * time.Minute)},
			"fresh": {tokens: 1, last: now},
		},
		idle: time.Minute,
	}
	mixed.sweepOnce()

	if _, ok := mixed.buckets["old"]; ok {
		t.Error("expired bucket kept, want removed")
	}

	if _, ok := mixed.buckets["fresh"]; !ok {
		t.Error("unexpired bucket removed, want kept")
	}
}

func TestCoverName(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	if got := s.Name(); got != "memory" {
		t.Errorf("Name() = %q, want %q", got, "memory")
	}
}
