package resilience_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

func TestOptions_zeroValue_valid(t *testing.T) {
	t.Parallel()

	if err := (resilience.Options{}).Validate(); err != nil {
		t.Fatalf("zero Options Validate err = %v, want nil", err)
	}
}

func TestOptions_negativeTimeout_invalid(t *testing.T) {
	t.Parallel()

	err := (resilience.Options{Timeout: -1}).Validate()
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("Timeout -1 err = %v, want ErrInvalidOptions", err)
	}

	var ioe resilience.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_negativeBulkhead_invalid(t *testing.T) {
	t.Parallel()

	cases := []resilience.Options{
		{Bulkhead: resilience.BulkheadOptions{MaxConcurrent: -1}},
		{Bulkhead: resilience.BulkheadOptions{MaxQueue: -1}},
		{Bulkhead: resilience.BulkheadOptions{MaxWait: -1}},
	}

	for i, o := range cases {
		if err := o.Validate(); !errors.Is(err, resilience.ErrInvalidOptions) {
			t.Errorf("case %d err = %v, want ErrInvalidOptions", i, err)
		}
	}
}

func TestOptions_breakerEnabled_ratioOutOfRange_invalid(t *testing.T) {
	t.Parallel()

	for _, ratio := range []float64{-0.1, 1.1} {
		o := resilience.Options{Breaker: resilience.BreakerOptions{Enabled: true, FailureRatio: ratio}}
		if err := o.Validate(); !errors.Is(err, resilience.ErrInvalidOptions) {
			t.Errorf("FailureRatio %v err = %v, want ErrInvalidOptions", ratio, err)
		}
	}
}

func TestOptions_breakerDisabled_ratioIgnored(t *testing.T) {
	t.Parallel()

	o := resilience.Options{Breaker: resilience.BreakerOptions{Enabled: false, FailureRatio: 2}}
	if err := o.Validate(); err != nil {
		t.Fatalf("disabled breaker bad ratio err = %v, want nil", err)
	}
}

func TestOptions_retryOutOfRange_invalid(t *testing.T) {
	t.Parallel()

	cases := []resilience.Options{
		{Retry: retry.Policy{BaseDelay: -1}},
		{Retry: retry.Policy{MaxDelay: -1}},
		{Retry: retry.Policy{MaxAttempts: -1}},
		{Retry: retry.Policy{Jitter: -0.1}},
		{Retry: retry.Policy{Jitter: 1.1}},
	}

	for i, o := range cases {
		if err := o.Validate(); !errors.Is(err, resilience.ErrInvalidOptions) {
			t.Errorf("case %d err = %v, want ErrInvalidOptions", i, err)
		}
	}
}

func TestOptions_joinsViolations(t *testing.T) {
	t.Parallel()

	err := (resilience.Options{Timeout: -1, Bulkhead: resilience.BulkheadOptions{MaxConcurrent: -1}}).Validate()
	if !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("joined err = %v, want ErrInvalidOptions", err)
	}

	var ioe resilience.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestDefault_valid(t *testing.T) {
	t.Parallel()

	o := resilience.Default()
	if err := o.Validate(); err != nil {
		t.Fatalf("Default() Validate err = %v", err)
	}

	if !o.Breaker.Enabled {
		t.Error("Default() breaker Enabled = false, want true")
	}

	if o.Timeout != resilience.DefaultTimeout {
		t.Errorf("Default() Timeout = %v, want %v", o.Timeout, resilience.DefaultTimeout)
	}

	if o.Bulkhead.MaxConcurrent != resilience.DefaultMaxConcurrent {
		t.Errorf("Default() MaxConcurrent = %d, want %d", o.Bulkhead.MaxConcurrent, resilience.DefaultMaxConcurrent)
	}
}
