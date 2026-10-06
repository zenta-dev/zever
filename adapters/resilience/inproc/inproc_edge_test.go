package inproc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestReadyToTrip_consecutiveFailures(t *testing.T) {
	t.Parallel()

	rt := readyToTrip(resilience.BreakerOptions{ConsecutiveFailures: 3})

	if rt(gobreaker.Counts{ConsecutiveFailures: 2}) {
		t.Error("ReadyToTrip(2 failures) = true, want false")
	}

	if !rt(gobreaker.Counts{ConsecutiveFailures: 3}) {
		t.Error("ReadyToTrip(3 failures) = true, want false")
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

	if got := mapBreakerError(gobreaker.ErrOpenState); !errors.Is(got, resilience.ErrOpenState) {
		t.Errorf("map(ErrOpenState) = %v, want ErrOpenState", got)
	}

	if got := mapBreakerError(gobreaker.ErrTooManyRequests); !errors.Is(got, resilience.ErrTooManyRequests) {
		t.Errorf("map(ErrTooManyRequests) = %v, want ErrTooManyRequests", got)
	}

	sentinel := errors.New("boom")
	if got := mapBreakerError(sentinel); !errors.Is(got, sentinel) {
		t.Errorf("map(other) = %v, want passthrough", got)
	}

	if got := mapBreakerError(nil); got != nil {
		t.Errorf("map(nil) = %v, want nil", got)
	}
}

func TestAcquire_queueLimit(t *testing.T) {
	t.Parallel()

	g := newGuard("queue", resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 1, MaxQueue: 1, MaxWait: time.Second},
	})

	g.waiting.Store(1)

	if err := g.acquire(t.Context()); !errors.Is(err, resilience.ErrBulkheadFull) {
		t.Fatalf("acquire(at queue limit) error = %v, want ErrBulkheadFull", err)
	}
}

func TestExecute_afterClose(t *testing.T) {
	t.Parallel()

	g := newGuard("closed", resilience.Options{})

	if err := g.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrClosed) {
		t.Fatalf("Execute(after close) error = %v, want ErrClosed", err)
	}
}

func TestGuardClose_idempotent(t *testing.T) {
	t.Parallel()

	g := newGuard("idem", resilience.Options{})

	if err := g.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestManagerClose_idempotent(t *testing.T) {
	t.Parallel()

	m, err := New(resilience.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := m.Guard("dep"); err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
