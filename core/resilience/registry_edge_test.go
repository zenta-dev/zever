package resilience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestOpen_invalidOptionsPrecedenceOverUnknown(t *testing.T) {
	t.Parallel()

	_, err := resilience.Open(resilience.Adapter("registry-edge-missing"), resilience.Options{Timeout: -1})
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("Open(invalid+unknown) err = %v, want ErrInvalidOptions", err)
	}
}

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

func TestErrorUnwrapChains(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"duplicate", resilience.DuplicateError{Adapter: resilience.Memory}, resilience.ErrDuplicateAdapter},
		{"unknown", resilience.UnknownAdapterError{Adapter: resilience.Memory}, resilience.ErrUnknownAdapter},
		{"invalid_adapter", resilience.InvalidAdapterError{Adapter: "x"}, resilience.ErrInvalidAdapter},
		{"invalid_options", resilience.InvalidOptionsError{Reason: "x"}, resilience.ErrInvalidOptions},
	}

	for _, c := range cases {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s errors.Is = false, want true for %v", c.name, c.want)
		}
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

func TestAdapterString_custom(t *testing.T) {
	t.Parallel()

	if got := resilience.Adapter("custom-edge").String(); got != "custom-edge" {
		t.Errorf("String() = %q, want custom-edge", got)
	}
}

func TestRegister_duplicateKeepsFirstFactory(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	first := func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }
	second := func(resilience.Options) (resilience.Manager, error) {
		return nil, errors.New("registry-edge: must not run")
	}

	if err := resilience.Register(a, first); err != nil {
		t.Fatalf("first Register err = %v", err)
	}

	if err := resilience.Register(a, second); !errors.Is(err, resilience.ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}

	m, err := resilience.Open(a, resilience.Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want first factory", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
