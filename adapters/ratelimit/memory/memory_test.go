package memory_test

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/ratelimit/memory"
)

func newLimiter(t *testing.T, opts ratelimit.Options) ratelimit.Limiter {
	t.Helper()

	l, err := memory.New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	return l
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

func TestBurstThenDeny(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 10, Burst: 3})
	ctx := t.Context()

	for i := 0; i < 3; i++ {
		d, err := l.Allow(ctx, "k-burst", 1)
		if err != nil {
			t.Fatalf("Allow %d failed: %v", i, err)
		}

		if !d.Allowed {
			t.Fatalf("Allow %d denied, want allowed", i)
		}
	}

	d, err := l.Allow(ctx, "k-burst", 1)
	if err != nil {
		t.Fatalf("4th Allow failed: %v", err)
	}

	if d.Allowed {
		t.Fatal("4th Allow allowed, want denied")
	}

	if d.RetryAfter <= 0 {
		t.Fatalf("RetryAfter = %v, want > 0", d.RetryAfter)
	}
}

func TestRefill(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 100, Burst: 3})
	ctx := t.Context()

	for i := 0; i < 3; i++ {
		d, err := l.Allow(ctx, "k-refill", 1)
		if err != nil || !d.Allowed {
			t.Fatalf("Allow %d = %+v, err = %v, want allowed", i, d, err)
		}
	}

	d, err := l.Allow(ctx, "k-refill", 1)
	if err != nil {
		t.Fatalf("drain Allow failed: %v", err)
	}

	if d.Allowed {
		t.Fatal("drained Allow allowed, want denied")
	}

	// Rate 100/s refills one token per 10ms: poll until the refill lands.
	eventually(t, 2*time.Second, func() bool {
		d, err := l.Allow(ctx, "k-refill", 1)
		return err == nil && d.Allowed
	}, "token refill")
}

func TestInvalidCost(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 10, Burst: 3})
	ctx := t.Context()

	for _, cost := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := l.Allow(ctx, "k-cost", cost); !errors.Is(err, ratelimit.ErrInvalidCost) {
			t.Errorf("Allow(cost=%v) err = %v, want ErrInvalidCost", cost, err)
		}
	}
}

func TestInvalidKey(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 10, Burst: 3})
	ctx := t.Context()

	for _, key := range []string{"", string(make([]byte, ratelimit.MaxKeyLen+1)), "a\nb", "a\x00b"} {
		if _, err := l.Allow(ctx, key, 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
			t.Errorf("Allow(key len %d) err = %v, want ErrInvalidKey", len(key), err)
		}

		if err := l.Reset(ctx, key); !errors.Is(err, ratelimit.ErrInvalidKey) {
			t.Errorf("Reset(key len %d) err = %v, want ErrInvalidKey", len(key), err)
		}
	}
}

func TestResetRestoresFull(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 1, Burst: 2})
	ctx := t.Context()

	_, _ = l.Allow(ctx, "k-reset", 1)
	_, _ = l.Allow(ctx, "k-reset", 1)

	d, _ := l.Allow(ctx, "k-reset", 1)
	if d.Allowed {
		t.Fatal("expected denial before reset")
	}

	if err := l.Reset(ctx, "k-reset"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	d, err := l.Allow(ctx, "k-reset", 2)
	if err != nil || !d.Allowed {
		t.Fatalf("post-reset Allow = %+v, err = %v, want allowed", d, err)
	}
}

func TestResetUnknownNil(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 10, Burst: 3})

	if err := l.Reset(t.Context(), "k-missing"); err != nil {
		t.Fatalf("Reset unknown = %v, want nil", err)
	}
}

func TestIdleExpiry(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 1, Burst: 2, IdleTTL: 50 * time.Millisecond})
	ctx := t.Context()

	_, _ = l.Allow(ctx, "k-idle", 2)

	d, _ := l.Allow(ctx, "k-idle", 1)
	if d.Allowed {
		t.Fatal("expected denial before idle expiry")
	}

	// IdleTTL 50ms: poll until the idle bucket is reclaimed as fresh.
	eventually(t, 2*time.Second, func() bool {
		d, err := l.Allow(ctx, "k-idle", 2)
		return err == nil && d.Allowed
	}, "idle expiry")
}

func TestAfterClose(t *testing.T) {
	t.Parallel()

	l, err := memory.New(ratelimit.Options{Rate: 10, Burst: 3})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := l.Allow(t.Context(), "k", 1); !errors.Is(err, ratelimit.ErrClosed) {
		t.Errorf("Allow after close = %v, want ErrClosed", err)
	}

	if err := l.Reset(t.Context(), "k"); !errors.Is(err, ratelimit.ErrClosed) {
		t.Errorf("Reset after close = %v, want ErrClosed", err)
	}

	if err := l.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestConcurrentSameKey(t *testing.T) {
	t.Parallel()

	l := newLimiter(t, ratelimit.Options{Rate: 10, Burst: 5})
	ctx := t.Context()

	const n = 50

	var wg sync.WaitGroup

	allowed := make(chan int, n)

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			d, err := l.Allow(ctx, "k-conc", 1)
			if err != nil {
				t.Errorf("Allow failed: %v", err)
				return
			}

			if d.Allowed {
				allowed <- 1
			}
		}()
	}

	wg.Wait()
	close(allowed)

	total := 0
	for range allowed {
		total++
	}

	if total > 5 {
		t.Fatalf("allowed %d > burst 5", total)
	}
}

func TestSweepStopsOnClose(t *testing.T) {
	t.Parallel()

	l, err := memory.New(ratelimit.Options{Rate: 10, Burst: 3, SweepInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	done := make(chan error, 1)

	go func() { done <- l.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return promptly, sweeper leak?")
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := memory.New(ratelimit.Options{})
	if err == nil {
		t.Fatal("New(empty) = nil, want error")
	}
	if !errors.Is(err, ratelimit.ErrInvalidOptions) {
		t.Errorf("New(empty) err = %v, want ErrInvalidOptions", err)
	}
	if !strings.HasPrefix(err.Error(), "memory: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "memory: ")
	}
}
