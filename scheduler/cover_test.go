package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCoverUnknownAdapterErrorString(t *testing.T) {
	t.Parallel()

	err := &UnknownAdapterError{Adapter: Adapter(999)}
	if got, want := err.Error(), "scheduler: unknown adapter: unknown"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCoverInvalidAdapterErrorString(t *testing.T) {
	t.Parallel()

	err := &InvalidAdapterError{Adapter: "bogus"}
	if got, want := err.Error(), `scheduler: invalid adapter: "bogus"`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCoverInvalidSpecErrorString(t *testing.T) {
	t.Parallel()

	cause := errors.New("parse boom")
	err := &InvalidSpecError{Spec: "bogus", Err: cause}
	if got, want := err.Error(), `scheduler: invalid spec "bogus": parse boom`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCoverInvalidSpecUnwrapNilCause(t *testing.T) {
	t.Parallel()

	err := &InvalidSpecError{Spec: "bogus"}
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}

	unwrapped := err.Unwrap()
	if len(unwrapped) != 1 || !errors.Is(unwrapped[0], ErrInvalidSpec) {
		t.Fatalf("Unwrap() = %v, want [ErrInvalidSpec]", unwrapped)
	}
}

func TestCoverStopTimeout(t *testing.T) {
	t.Parallel()

	// runDone never closes: Stop must give up at closeTimeout.
	e := &embedded{
		cancel:       func() {},
		runDone:      make(chan struct{}),
		closeTimeout: 20 * time.Millisecond,
		started:      true,
	}

	start := time.Now()
	err := e.Stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took %v, want ~20ms", elapsed)
	}
}
