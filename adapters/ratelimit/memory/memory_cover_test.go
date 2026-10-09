package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// bareStore builds a store with initialized stripes but no background
// janitor, for white-box sweep tests.
func bareStore(idle time.Duration) *store {
	s := &store{idle: idle, stripes: make([]stripe, DefaultStripeCount)}
	for i := range s.stripes {
		s.stripes[i].buckets = make(map[string]*bucket)
	}
	return s
}

// setBuckets inserts m into the owning stripes.
func setBuckets(t *testing.T, s *store, m map[string]*bucket) {
	t.Helper()
	for k, b := range m {
		st := s.stripeFor(k)
		st.mu.Lock()
		st.buckets[k] = b
		st.mu.Unlock()
	}
}

// getBucket returns the bucket for key, or nil.
func getBucket(t *testing.T, s *store, key string) *bucket {
	t.Helper()
	st := s.stripeFor(key)
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.buckets[key]
}

// countBuckets sums live buckets across all stripes.
func countBuckets(s *store) int {
	n := 0
	for i := range s.stripes {
		st := &s.stripes[i]
		st.mu.RLock()
		n += len(st.buckets)
		st.mu.RUnlock()
	}
	return n
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

	s.stripeFor("k-postlock").mu.Lock()

	type result struct {
		err error
	}

	done := make(chan result, 1)

	go func() {
		_, err := s.Allow(t.Context(), "k-postlock", 1)
		done <- result{err: err}
	}()

	// The test holds the stripe lock, so the goroutine blocks on it after
	// the pre-lock checks; closing underneath exercises the post-lock path
	// deterministically without any timing wait.
	s.closed.Store(true)
	s.stripeFor("k-postlock").mu.Unlock()

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
	st := s.stripeFor("k-cap")
	st.mu.Lock()
	st.buckets["k-cap"].last = time.Now().Add(-300 * time.Millisecond)
	st.mu.Unlock()

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

	st := s.stripeFor("k-idle")
	st.mu.Lock()
	st.buckets["k-idle"].last = time.Now().Add(-2 * s.idle)
	st.mu.Unlock()

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

	st := s.stripeFor("k-future")
	st.mu.Lock()
	st.buckets["k-future"].last = time.Now().Add(time.Second)
	st.mu.Unlock()

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

	if b := getBucket(t, s, "k-del"); b != nil {
		t.Fatal("bucket still present after Reset")
	}

	d, err := s.Allow(ctx, "k-del", 5)
	if err != nil || !d.Allowed {
		t.Fatalf("post-reset Allow = %+v, err = %v, want allowed", d, err)
	}
}

func TestCoverResetPostLockClosed(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})

	s.stripeFor("k-postlock").mu.Lock()

	done := make(chan error, 1)

	go func() {
		done <- s.Reset(t.Context(), "k-postlock")
	}()

	// Same as Allow post-lock: the goroutine parks on the stripe lock,
	// which this test holds, so close underneath without any timing wait.
	s.closed.Store(true)
	s.stripeFor("k-postlock").mu.Unlock()

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

	if n := countBuckets(s); n != 0 {
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

	st := s.stripeFor("k-sweep")
	st.mu.Lock()
	st.buckets["k-sweep"].last = time.Now().Add(-time.Second)
	st.mu.Unlock()

	eventually(t, 2*time.Second, func() bool {
		return getBucket(t, s, "k-sweep") == nil
	}, "background sweeper to remove expired bucket")

	if closeErr := l.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
}

func TestCoverSweepOnceTable(t *testing.T) {
	now := time.Now()

	empty := bareStore(time.Minute)
	empty.sweepOnce()

	if n := countBuckets(empty); n != 0 {
		t.Errorf("empty sweep buckets = %d, want 0", n)
	}

	kept := bareStore(time.Minute)
	setBuckets(t, kept, map[string]*bucket{"fresh": {tokens: 1, last: now}})
	kept.sweepOnce()

	if b := getBucket(t, kept, "fresh"); b == nil {
		t.Error("unexpired bucket removed, want kept")
	}

	mixed := bareStore(time.Minute)
	setBuckets(t, mixed, map[string]*bucket{
		"old":   {tokens: 0, last: now.Add(-2 * time.Minute)},
		"fresh": {tokens: 1, last: now},
	})
	mixed.sweepOnce()

	if b := getBucket(t, mixed, "old"); b != nil {
		t.Error("expired bucket kept, want removed")
	}

	if b := getBucket(t, mixed, "fresh"); b == nil {
		t.Error("unexpired bucket removed, want kept")
	}
}

func TestCoverName(t *testing.T) {
	s := newCoverStore(t, ratelimit.Options{Rate: 10, Burst: 5})
	if got := s.Name(); got != "memory" {
		t.Errorf("Name() = %q, want %q", got, "memory")
	}
}

func TestCoverMaxEntriesBoundHeldUnderConcurrency(t *testing.T) {
	t.Parallel()

	l, err := New(ratelimit.Options{Rate: 10, Burst: 1, MaxEntries: 8})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, ok := l.(*store)
	if !ok {
		t.Fatalf("New returned %T, want *store", l)
	}

	t.Cleanup(func() { _ = l.Close() })

	const (
		goroutines = 16
		keysPer    = 32
	)

	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			for i := 0; i < keysPer; i++ {
				key := fmt.Sprintf("g%d-k%d", g, i)
				if _, err := l.Allow(t.Context(), key, 1); err != nil {
					t.Errorf("Allow(%s) failed: %v", key, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()

	if got := s.total.Load(); got > 8 {
		t.Fatalf("total = %d, want <= 8 (MaxEntries bound)", got)
	}

	if got := s.total.Load(); got != 8 {
		t.Fatalf("total = %d, want 8 (table full)", got)
	}

	if got := countBuckets(s); got != 8 {
		t.Fatalf("live buckets = %d, want 8", got)
	}
}

// TestCoverMaxEntriesBoundStrictUnderContention hammers a tiny table with
// far more concurrent distinct keys than MaxEntries. Reservations used to
// be invisible to eviction while in flight, so a burst of concurrent
// admissions could overshoot the bound before their inserts landed.
func TestCoverMaxEntriesBoundStrictUnderContention(t *testing.T) {
	t.Parallel()

	l, err := New(ratelimit.Options{Rate: 1000, Burst: 1000, MaxEntries: 2})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, ok := l.(*store)
	if !ok {
		t.Fatalf("New returned %T, want *store", l)
	}

	t.Cleanup(func() { _ = l.Close() })

	const (
		goroutines = 64
		keysPer    = 64
	)

	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			for i := 0; i < keysPer; i++ {
				key := fmt.Sprintf("g%d-k%d", g, i)
				if _, err := l.Allow(t.Context(), key, 1); err != nil {
					t.Errorf("Allow(%s) failed: %v", key, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()

	if got := s.total.Load(); got > 2 {
		t.Fatalf("total = %d, want <= 2 (MaxEntries bound)", got)
	}

	if got := countBuckets(s); got > 2 {
		t.Fatalf("live buckets = %d, want <= 2", got)
	}
}
