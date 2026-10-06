// Package resiliencetest provides the conformance kit third-party resilience adapters run to prove backend parity.
package resiliencetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

// OpenFunc builds a Manager for opts. Implementations should fail the test on
// construction error and register a cleanup that closes the Manager.
type OpenFunc func(t *testing.T, opts resilience.Options) resilience.Manager

// Conformance verifies factory-built managers implement the resilience
// contract: registry round-trips, a closed breaker admitting calls, a breaker
// that trips, fails fast, and recovers through a half-open probe, a bulkhead
// that rejects past its limit, an overall timeout, and caller cancellation that
// never trips the breaker. Each subtest takes a fresh Manager from open so cases
// stay isolated. The breaker-recovery subtest crosses a short open timeout with
// a real timer; it never uses sleeps as a synchronization primitive.
func Conformance(t *testing.T, open OpenFunc) {
	t.Helper()

	t.Run("OpenRegister", conformanceOpenRegister)
	t.Run("ClosedAllows", func(t *testing.T) { conformanceClosedAllows(t, open) })
	t.Run("BreakerTrips", func(t *testing.T) { conformanceBreakerTrips(t, open) })
	t.Run("Retry", func(t *testing.T) { conformanceRetry(t, open) })
	t.Run("BulkheadRejects", func(t *testing.T) { conformanceBulkheadRejects(t, open) })
	t.Run("Timeout", func(t *testing.T) { conformanceTimeout(t, open) })
	t.Run("CallerCancellation", func(t *testing.T) { conformanceCallerCancellation(t, open) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, open) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := resilience.Open(resilience.Adapter("conformance-missing-adapter"), resilience.Options{}); !errors.Is(err, resilience.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := resilience.Adapter("conformance-probe-resilience")

	if err := resilience.Register(probe, nil); !errors.Is(err, resilience.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(resilience.Options) (resilience.Manager, error) {
		return nil, errors.New("resiliencetest: probe factory must not run")
	}

	_ = resilience.Register(probe, stub)

	if err := resilience.Register(probe, stub); !errors.Is(err, resilience.ErrDuplicateAdapter) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicateAdapter", err)
	}
}

func conformanceClosedAllows(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{})

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if g.Name() != "dep" {
		t.Errorf("Name() = %q, want dep", g.Name())
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed", g.State())
	}

	if execErr := g.Execute(t.Context(), func(context.Context) error { return nil }); execErr != nil {
		t.Errorf("Execute(success) error = %v, want nil", execErr)
	}

	again, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard(same) error = %v", err)
	}

	if again != g {
		t.Error("Guard(same) returned a different Guard, want the cached one")
	}
}

func conformanceBreakerTrips(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{
		Breaker: resilience.BreakerOptions{
			Enabled:      true,
			MinRequests:  2,
			FailureRatio: 0.5,
			Timeout:      40 * time.Millisecond,
		},
	})

	g, err := m.Guard("breaker")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	boom := errors.New("resiliencetest: boom")

	for i := 0; i < 2; i++ {
		if err := g.Execute(t.Context(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("Execute(fail) error = %v, want boom", err)
		}
	}

	if g.State() != resilience.StateOpen {
		t.Fatalf("State() = %s, want open", g.State())
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrOpenState) {
		t.Fatalf("Execute(open) error = %v, want ErrOpenState", err)
	}

	// Cross the open timeout so the breaker half-opens; this is the feature
	// under test, not a synchronization primitive.
	time.Sleep(60 * time.Millisecond)

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("half-open probe error = %v, want nil", err)
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() after probe = %s, want closed", g.State())
	}
}

func conformanceRetry(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{
		Retry: retry.Policy{MaxAttempts: 3},
	})

	g, err := m.Guard("retry")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	var calls int

	boom := errors.New("resiliencetest: boom")

	err = g.Execute(t.Context(), func(context.Context) error {
		calls++
		if calls < 3 {
			return boom
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Execute(retry) error = %v, want nil", err)
	}

	if calls != 3 {
		t.Errorf("fn calls = %d, want 3", calls)
	}

	calls = 0

	err = g.Execute(t.Context(), func(context.Context) error {
		calls++
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) error = %v, want context.Canceled", err)
	}

	if calls != 1 {
		t.Errorf("fn calls on cancellation = %d, want 1 (no retry)", calls)
	}
}

func conformanceBulkheadRejects(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 1, MaxWait: 30 * time.Millisecond},
	})

	g, err := m.Guard("bulkhead")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		_ = g.Execute(t.Context(), func(context.Context) error {
			close(entered)
			<-release

			return nil
		})
	}()

	<-entered

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrBulkheadFull) {
		t.Errorf("Execute(full) error = %v, want ErrBulkheadFull", err)
	}

	close(release)
	wg.Wait()
}

func conformanceTimeout(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{Timeout: 30 * time.Millisecond})

	g, err := m.Guard("timeout")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	err = g.Execute(t.Context(), func(ctx context.Context) error {
		<-ctx.Done()

		return ctx.Err()
	})
	if !errors.Is(err, resilience.ErrTimeout) {
		t.Errorf("Execute(slow) error = %v, want ErrTimeout", err)
	}
}

func conformanceCallerCancellation(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{
		Breaker: resilience.BreakerOptions{
			Enabled:      true,
			MinRequests:  1,
			FailureRatio: 0.1,
			Timeout:      time.Second,
		},
	})

	g, err := m.Guard("cancel")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		err := g.Execute(t.Context(), func(context.Context) error { return context.Canceled })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute(canceled) error = %v, want context.Canceled", err)
		}
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed (caller cancellation must not trip the breaker)", g.State())
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := g.Execute(ctx, func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("Execute(pre-canceled) error = %v, want context.Canceled", err)
	}
}

func conformanceClose(t *testing.T, open OpenFunc) {
	t.Helper()

	m := open(t, resilience.Options{})

	if _, err := m.Guard("close"); err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Errorf("second Close() error = %v, want nil", err)
	}

	if _, err := m.Guard("after-close"); !errors.Is(err, resilience.ErrClosed) {
		t.Errorf("Guard(after close) error = %v, want ErrClosed", err)
	}
}
