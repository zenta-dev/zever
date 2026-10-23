package resilience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestDo_cancelledContext(t *testing.T) {
	t.Parallel()

	g := &stubGuard{name: "dep"}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := resilience.Do(ctx, g, func(ctx context.Context) (int, error) {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}

		return 42, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do(canceled) err = %v, want context.Canceled", err)
	}

	if got != 0 {
		t.Fatalf("Do(canceled) value = %d, want zero", got)
	}
}

func TestDo_fnErrorZeroValue(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("registry-edge: boom")
	g := &stubGuard{name: "dep"}

	got, err := resilience.Do(t.Context(), g, func(context.Context) (string, error) {
		return "nonzero", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do err = %v, want sentinel", err)
	}

	if got != "" {
		t.Fatalf("Do value = %q, want zero on fn error", got)
	}
}

func TestDefault_matchesConsts(t *testing.T) {
	t.Parallel()

	o := resilience.Default()
	if err := o.Validate(); err != nil {
		t.Fatalf("Default() Validate err = %v, want nil", err)
	}

	if o.Timeout != resilience.DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", o.Timeout, resilience.DefaultTimeout)
	}

	if o.Retry.MaxAttempts != resilience.DefaultRetryMaxAttempts {
		t.Errorf("Retry.MaxAttempts = %d, want %d", o.Retry.MaxAttempts, resilience.DefaultRetryMaxAttempts)
	}

	if o.Breaker.MinRequests != resilience.DefaultBreakerMinRequests {
		t.Errorf("Breaker.MinRequests = %d, want %d", o.Breaker.MinRequests, resilience.DefaultBreakerMinRequests)
	}

	if o.Bulkhead.MaxWait != resilience.DefaultMaxWait {
		t.Errorf("Bulkhead.MaxWait = %v, want %v", o.Bulkhead.MaxWait, resilience.DefaultMaxWait)
	}
}

func TestValidate_jitterBoundariesValid(t *testing.T) {
	t.Parallel()

	for _, j := range []float64{0, 1} {
		o := resilience.Default()
		o.Retry.Jitter = j

		if err := o.Validate(); err != nil {
			t.Errorf("Jitter %v err = %v, want nil", j, err)
		}
	}
}
