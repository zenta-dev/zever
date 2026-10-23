package auth

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers the registry typed errors left uncovered
// when the adapter registry was removed: Register/Open used to exercise
// them via duplicate/unknown/invalid adapter paths, so direct unit tests
// now pin the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "jwt"}, ErrDuplicateAdapter, "jwt", &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "jwt"}, ErrUnknownAdapter, "jwt", &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "jwt"}, ErrInvalidAdapter, "jwt", &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad ttl"}, ErrInvalidOptions, "bad ttl", &InvalidOptionsError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "auth: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "auth: ")
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
}
