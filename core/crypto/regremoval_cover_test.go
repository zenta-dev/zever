package crypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// TestRegRemovalTypedErrors covers the typed errors left uncovered when the
// adapter registry was removed: Register/Open used to exercise them via
// duplicate/unknown/invalid adapter paths, so direct unit tests now pin the
// message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"invalid", crypto.InvalidAdapterError{Adapter: "bogus"}, crypto.ErrInvalidAdapter, "bogus", &crypto.InvalidAdapterError{}},
		{"duplicate", crypto.DuplicateAdapterError{Adapter: "aead"}, crypto.ErrDuplicateAdapter, "aead", &crypto.DuplicateAdapterError{}},
		{"unknown", crypto.UnknownAdapterError{Adapter: "aead"}, crypto.ErrUnknownAdapter, "aead", &crypto.UnknownAdapterError{}},
		{"options", crypto.InvalidOptionsError{Reason: "bad key"}, crypto.ErrInvalidOptions, "bad key", &crypto.InvalidOptionsError{}},
		{"devkey", crypto.DevKeyError{}, crypto.ErrDevKey, "dev key", &crypto.DevKeyError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "crypto: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "crypto: ")
			}

			if !strings.Contains(strings.ToLower(msg), tc.wantSub) {
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
