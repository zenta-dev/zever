package cdn

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers every typed error: the adapter registry
// used to exercise these via Register/Open paths, so direct unit tests now
// pin the message constructors, sentinel unwrapping, and As round-trips.
// Prefix-only assertions keep the test off exact wording.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, "bogus", &InvalidAdapterError{}},
		{"duplicate", DuplicateError{Adapter: "akamai"}, ErrDuplicateAdapter, "akamai", &DuplicateError{}},
		{"unknown", UnknownAdapterError{Adapter: "akamai"}, ErrUnknownAdapter, "akamai", &UnknownAdapterError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "cdn: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "cdn: ")
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

			type unwrapper interface{ Unwrap() error }

			u, ok := tc.err.(unwrapper)
			if !ok {
				t.Fatalf("%T has no Unwrap", tc.err)
			}

			if got := u.Unwrap(); got != tc.sentinel {
				t.Errorf("Unwrap() = %v, want sentinel", got)
			}
		})
	}
}
