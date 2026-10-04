package ai

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestEdgeOptions_Validate_extremeTimeout(t *testing.T) {
	t.Parallel()

	if err := (Options{Timeout: time.Duration(math.MaxInt64)}).Validate(); err != nil {
		t.Fatalf("max timeout err = %v, want nil", err)
	}

	if err := (Options{Timeout: time.Duration(math.MinInt64)}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("min timeout err = %v, want ErrInvalidOptions", err)
	}
}

func TestEdgeOpen_reopenAfterClose(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	first, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if closeErr := first.Close(); closeErr != nil {
		t.Fatalf("Close err = %v", closeErr)
	}

	second, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("re-Open err = %v", err)
	}

	if second == nil {
		t.Fatal("re-Open returned nil")
	}
}
