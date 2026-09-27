package scheduler

import (
	"errors"
	"testing"
)

func TestSentinels(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		ErrNilFactory,
		ErrDuplicate,
		ErrUnknownAdapter,
		ErrInvalidAdapter,
		ErrInvalidOptions,
		ErrInvalidSpec,
	} {
		if err == nil || err.Error() == "" {
			t.Fatalf("sentinel %v empty", err)
		}
	}
}

func TestDuplicateAdapterErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &DuplicateAdapterError{Adapter: Embedded}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err=%v want ErrDuplicate", err)
	}

	if got, want := err.Error(), `scheduler: duplicate registration: embedded`; got != want {
		t.Fatalf("Error()=%q want %q", got, want)
	}
}

func TestUnknownAdapterErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &UnknownAdapterError{Adapter: Adapter("")}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("err=%v want ErrUnknownAdapter", err)
	}
}

func TestInvalidAdapterErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidAdapterError{Adapter: "bogus"}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("err=%v want ErrInvalidAdapter", err)
	}
}

func TestInvalidOptionsErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidOptionsError{Reason: "dispatcher_is_required"}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
}

func TestInvalidSpecErrorUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("parse boom")
	err := &InvalidSpecError{Spec: "bogus", Err: cause}

	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("err=%v want ErrInvalidSpec", err)
	}

	if !errors.Is(err, cause) {
		t.Fatalf("err=%v want wrapped cause", err)
	}

	var spec *InvalidSpecError
	if !errors.As(err, &spec) {
		t.Fatalf("err=%v want InvalidSpecError", err)
	}
}
