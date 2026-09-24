package scheduler

import (
	"errors"
	"testing"
)

func TestCoverUnknownAdapterErrorString(t *testing.T) {
	t.Parallel()

	err := &UnknownAdapterError{Adapter: Adapter(999)}
	if got, want := err.Error(), "scheduler: unknown adapter: unknown (forgotten import?)"; got != want {
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
