package lock_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/lock"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrNotHeld":        {lock.ErrNotHeld.Error(), "lock: lock is no longer held"},
		"ErrNilFactory":     {lock.ErrNilFactory.Error(), "lock: nil factory"},
		"ErrDuplicate":      {lock.ErrDuplicate.Error(), "lock: duplicate registration"},
		"ErrUnknownAdapter": {lock.ErrUnknownAdapter.Error(), "lock: unknown adapter"},
		"ErrInvalidAdapter": {lock.ErrInvalidAdapter.Error(), "lock: invalid adapter"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestTypedErrorMessages_unwrap(t *testing.T) {
	t.Parallel()

	t.Run("invalid adapter", func(t *testing.T) {
		t.Parallel()

		err := lock.InvalidAdapterError{Adapter: "bogus"}
		if got, want := err.Error(), `lock: invalid adapter: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, lock.ErrInvalidAdapter) {
			t.Error("errors.Is(err, ErrInvalidAdapter) = false")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		err := &lock.DuplicateAdapterError{Adapter: lock.Memory}
		if got, want := err.Error(), `lock: duplicate registration: memory`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, lock.ErrDuplicate) {
			t.Error("errors.Is(err, ErrDuplicate) = false")
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		err := &lock.UnknownAdapterError{Adapter: lock.Adapter("")}
		if got, want := err.Error(), `lock: unknown adapter: unknown (forgotten import?)`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, lock.ErrUnknownAdapter) {
			t.Error("errors.Is(err, ErrUnknownAdapter) = false")
		}
	})
}
