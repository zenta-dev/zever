package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

// stubStore is an in-memory SharedDataStore so breaker-enabled guards can be
// built without Redis.
type stubStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *stubStore) Lock(string) error   { return nil }
func (s *stubStore) Unlock(string) error { return nil }

func (s *stubStore) GetData(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.data[name]
	if !ok {
		return nil, gobreaker.ErrNoSharedState
	}

	return d, nil
}

func (s *stubStore) SetData(name string, d []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string][]byte)
	}

	s.data[name] = d

	return nil
}

func mustInternalGuard(t *testing.T, name string, opts resilience.Options) *guard {
	t.Helper()

	g, err := newGuard(name, opts, resilience.DefaultRedisPrefix, &stubStore{})
	if err != nil {
		t.Fatalf("newGuard() error = %v", err)
	}

	return g
}

func TestReadyToTrip_consecutiveFailures(t *testing.T) {
	t.Parallel()

	rt := readyToTrip(resilience.BreakerOptions{ConsecutiveFailures: 3})

	if rt(gobreaker.Counts{ConsecutiveFailures: 2}) {
		t.Error("ReadyToTrip(2 failures) = true, want false")
	}

	if !rt(gobreaker.Counts{ConsecutiveFailures: 3}) {
		t.Error("ReadyToTrip(3 failures) = false, want true")
	}
}

func TestReadyToTrip_failureRatio(t *testing.T) {
	t.Parallel()

	rt := readyToTrip(resilience.BreakerOptions{MinRequests: 4, FailureRatio: 0.5})

	if rt(gobreaker.Counts{Requests: 3, TotalFailures: 3}) {
		t.Error("ReadyToTrip(below min requests) = true, want false")
	}

	if rt(gobreaker.Counts{Requests: 4, TotalFailures: 1}) {
		t.Error("ReadyToTrip(ratio 0.25) = true, want false")
	}

	if !rt(gobreaker.Counts{Requests: 4, TotalFailures: 2}) {
		t.Error("ReadyToTrip(ratio 0.5) = false, want true")
	}
}

func TestStateFromBreaker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   gobreaker.State
		want resilience.State
	}{
		{gobreaker.StateClosed, resilience.StateClosed},
		{gobreaker.StateOpen, resilience.StateOpen},
		{gobreaker.StateHalfOpen, resilience.StateHalfOpen},
		{gobreaker.State(99), resilience.StateClosed},
	}

	for _, c := range cases {
		if got := stateFromBreaker(c.in); got != c.want {
			t.Errorf("stateFromBreaker(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestMapBreakerError(t *testing.T) {
	t.Parallel()

	if got := mapBreakerError(nil); got != nil {
		t.Errorf("map(nil) = %v, want nil", got)
	}

	if got := mapBreakerError(gobreaker.ErrOpenState); !errors.Is(got, resilience.ErrOpenState) {
		t.Errorf("map(ErrOpenState) = %v, want ErrOpenState", got)
	}

	if got := mapBreakerError(gobreaker.ErrTooManyRequests); !errors.Is(got, resilience.ErrTooManyRequests) {
		t.Errorf("map(ErrTooManyRequests) = %v, want ErrTooManyRequests", got)
	}

	if got := mapBreakerError(gobreaker.ErrNoSharedState); !errors.Is(got, ErrNoSharedState) {
		t.Errorf("map(ErrNoSharedState) = %v, want ErrNoSharedState", got)
	}

	sentinel := errors.New("redis: boom")
	if got := mapBreakerError(sentinel); !errors.Is(got, sentinel) {
		t.Errorf("map(other) = %v, want passthrough", got)
	}
}

func TestBreakerSettings_excludesCancellation(t *testing.T) {
	t.Parallel()

	s := breakerSettings("dep", resilience.BreakerOptions{}, nil)

	if !s.IsSuccessful(nil) {
		t.Error("IsSuccessful(nil) = false, want true")
	}

	if s.IsSuccessful(errors.New("boom")) {
		t.Error("IsSuccessful(boom) = true, want false")
	}

	if !s.IsExcluded(context.Canceled) {
		t.Error("IsExcluded(Canceled) = false, want true")
	}

	if !s.IsExcluded(context.DeadlineExceeded) {
		t.Error("IsExcluded(DeadlineExceeded) = false, want true")
	}

	if s.IsExcluded(errors.New("boom")) {
		t.Error("IsExcluded(boom) = true, want false")
	}

	// Nil OnStateChange must not panic when the breaker reports a transition.
	s.OnStateChange("dep", gobreaker.StateClosed, gobreaker.StateOpen)
}

func TestBreakerSettings_customIsSuccessful(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("redis: tolerated")
	s := breakerSettings("dep", resilience.BreakerOptions{
		IsSuccessful: func(err error) bool { return errors.Is(err, sentinel) },
	}, nil)

	if !s.IsSuccessful(sentinel) {
		t.Error("IsSuccessful(tolerated) = false, want true")
	}

	if s.IsSuccessful(errors.New("other")) {
		t.Error("IsSuccessful(other) = true, want false")
	}
}

func TestBreakerSettings_onStateChangeMapping(t *testing.T) {
	t.Parallel()

	var gotFrom, gotTo resilience.State

	s := breakerSettings("dep", resilience.BreakerOptions{}, func(_ string, from, to resilience.State) {
		gotFrom, gotTo = from, to
	})
	s.OnStateChange("dep", gobreaker.StateClosed, gobreaker.StateOpen)

	if gotFrom != resilience.StateClosed || gotTo != resilience.StateOpen {
		t.Fatalf("transition = %s->%s, want closed->open", gotFrom, gotTo)
	}
}

func TestNewGuard_breakerDisabled(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "dep", resilience.Options{})

	if g.cb != nil {
		t.Error("newGuard(disabled) built a breaker, want nil")
	}

	if g.sem != nil {
		t.Error("newGuard(no bulkhead) built a semaphore, want nil")
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed", g.State())
	}
}

func TestNewGuard_breakerEnabledNamespacesPrefix(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "dep", resilience.Options{
		Breaker: resilience.BreakerOptions{Enabled: true, MinRequests: 1, FailureRatio: 0.1},
	})

	if g.cb == nil {
		t.Fatal("newGuard(enabled) breaker = nil, want breaker")
	}

	boom := errors.New("redis: boom")

	if err := g.Execute(t.Context(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Execute(fail) error = %v, want boom", err)
	}

	if g.State() != resilience.StateOpen {
		t.Fatalf("State() = %s, want open", g.State())
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrOpenState) {
		t.Fatalf("Execute(open) error = %v, want ErrOpenState", err)
	}
}

func TestNewGuard_bulkheadWired(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "dep", resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 2, MaxQueue: 2, MaxWait: time.Second},
	})

	if g.sem == nil {
		t.Fatal("newGuard(bulkhead) semaphore = nil, want semaphore")
	}
}

func TestAcquire_queueLimit(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "queue", resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 1, MaxQueue: 1, MaxWait: time.Second},
	})
	g.waiting.Store(1)

	if err := g.acquire(t.Context()); !errors.Is(err, resilience.ErrBulkheadFull) {
		t.Fatalf("acquire(at queue limit) error = %v, want ErrBulkheadFull", err)
	}
}

func TestAcquire_cancelledContext(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "cancel", resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 1},
	})

	// A pre-cancelled waiter surfaces cancellation even with a free slot.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := g.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire(canceled) error = %v, want context.Canceled", err)
	}
}

func TestWithRetry_exhausted(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "retry", resilience.Options{
		Retry: retry.Policy{MaxAttempts: 2},
	})

	sentinel := errors.New("redis: boom")
	calls := 0

	err := g.withRetry(t.Context(), func(context.Context) error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("withRetry(exhausted) error = %v, want wrap of sentinel", err)
	}

	if calls != 2 {
		t.Errorf("fn calls = %d, want 2", calls)
	}
}

func TestWithRetry_zeroAttemptsMeansOne(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "retry", resilience.Options{})

	sentinel := errors.New("redis: boom")
	calls := 0

	err := g.withRetry(t.Context(), func(context.Context) error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("withRetry error = %v, want wrap of sentinel", err)
	}

	if calls != 1 {
		t.Errorf("fn calls = %d, want 1", calls)
	}
}

func TestWithRetry_cancelledContext(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "retry", resilience.Options{
		Retry: retry.Policy{MaxAttempts: 3},
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := g.withRetry(ctx, func(context.Context) error { return errors.New("boom") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("withRetry(canceled) error = %v, want context.Canceled", err)
	}
}

func TestSleepCtx(t *testing.T) {
	t.Parallel()

	if err := sleepCtx(t.Context(), 0); err != nil {
		t.Errorf("sleepCtx(live, 0) error = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := sleepCtx(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx(canceled, 0) error = %v, want context.Canceled", err)
	}

	if err := sleepCtx(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx(canceled, wait) error = %v, want context.Canceled", err)
	}
}

func TestGuardExecute_afterClose(t *testing.T) {
	t.Parallel()

	g := mustInternalGuard(t, "closed", resilience.Options{})

	if err := g.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrClosed) {
		t.Fatalf("Execute(after close) error = %v, want ErrClosed", err)
	}
}

func TestErrNoSharedState_message(t *testing.T) {
	t.Parallel()

	if got := ErrNoSharedState.Error(); got != "redis: no shared breaker state" {
		t.Errorf("ErrNoSharedState = %q, want redis-prefixed message", got)
	}

	if !errors.Is(mapBreakerError(gobreaker.ErrNoSharedState), ErrNoSharedState) {
		t.Error("mapBreakerError(ErrNoSharedState) does not match ErrNoSharedState")
	}
}
