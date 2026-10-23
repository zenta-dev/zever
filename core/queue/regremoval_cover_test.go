package queue

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers the typed errors left uncovered when the
// adapter registry was removed: Register/Open used to exercise them via
// duplicate/unknown/invalid adapter paths, so direct unit tests now pin the
// message constructors, sentinel unwrapping, and As round-trips. It also
// covers the EmptyError bare-topic branch and the InvalidMessageIDError
// nil-cause Unwrap arm.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	t.Run("adapter", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name     string
			err      error
			sentinel error
			wantSub  string
			asTarget any
		}{
			{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, "bogus", &InvalidAdapterError{}},
			{"duplicate", DuplicateAdapterError{Adapter: "redis"}, ErrDuplicateAdapter, "redis", &DuplicateAdapterError{}},
			{"unknown", UnknownAdapterError{Adapter: "redis"}, ErrUnknownAdapter, "redis", &UnknownAdapterError{}},
		}

		for _, tc := range cases {
			tc := tc

			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				msg := tc.err.Error()
				if !strings.HasPrefix(msg, "queue: ") {
					t.Errorf("Error() = %q, want prefix %q", msg, "queue: ")
				}

				if !strings.Contains(msg, tc.wantSub) {
					t.Errorf("Error() = %q, want substring %q", msg, tc.wantSub)
				}

				if !errors.Is(tc.err, tc.sentinel) {
					t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
				}

				if !errors.As(tc.err, tc.asTarget) {
					t.Errorf("errors.As(%T) = false", tc.err)
				}
			})
		}
	})

	t.Run("empty/bare-topic", func(t *testing.T) {
		t.Parallel()

		err := EmptyError{}
		if got := err.Error(); got != ErrEmpty.Error() {
			t.Errorf("EmptyError{}.Error() = %q, want %q", got, ErrEmpty.Error())
		}

		if !errors.Is(err, ErrEmpty) {
			t.Errorf("errors.Is(EmptyError{}, ErrEmpty) = false")
		}

		if !errors.Is(err, ErrEmpty) {
			t.Errorf("Unwrap() = %v, want ErrEmpty", err.Unwrap())
		}
	})

	t.Run("message-id/nil-cause", func(t *testing.T) {
		t.Parallel()

		err := InvalidMessageIDError{ID: "bogus"}
		msg := err.Error()

		if !strings.HasPrefix(msg, "queue: ") {
			t.Errorf("Error() = %q, want prefix %q", msg, "queue: ")
		}

		if !strings.Contains(msg, "bogus") {
			t.Errorf("Error() = %q, want id", msg)
		}

		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("errors.Is(err, ErrInvalidMessageID) = false")
		}

		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("Unwrap() = %v, want ErrInvalidMessageID", err.Unwrap())
		}

		var target InvalidMessageIDError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(err, *InvalidMessageIDError) = false")
		}
	})

	t.Run("message-id/with-cause", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("short")
		err := InvalidMessageIDError{ID: "bogus", Err: cause}

		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("errors.Is(err, ErrInvalidMessageID) = false")
		}

		if !errors.Is(err, cause) {
			t.Errorf("errors.Is(err, cause) = false")
		}

		if got := err.Error(); !strings.Contains(got, "short") {
			t.Errorf("Error() = %q, want cause", got)
		}
	})
}
