package password_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

// TestRegRemovalTypedUnwrap covers the Unwrap methods left uncovered when the
// adapter registry was removed: Register/Open used to exercise them via
// duplicate/unknown/invalid adapter paths. Message strings stay covered by
// TestTypedErrorStrings; this pins sentinel unwrapping and As round-trips.
func TestRegRemovalTypedUnwrap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		asTarget any
	}{
		{"invalid", password.InvalidAdapterError{Adapter: "bcrypt"}, password.ErrInvalidAdapter, &password.InvalidAdapterError{}},
		{"duplicate", password.DuplicateAdapterError{Adapter: "argon2id"}, password.ErrDuplicateAdapter, &password.DuplicateAdapterError{}},
		{"unknown", password.UnknownAdapterError{Adapter: "nope"}, password.ErrUnknownAdapter, &password.UnknownAdapterError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !strings.HasPrefix(tc.err.Error(), "password: ") {
				t.Errorf("Error() = %q, want prefix %q", tc.err.Error(), "password: ")
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
