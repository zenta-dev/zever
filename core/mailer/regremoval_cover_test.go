package mailer

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
		wantSubs []string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "smtp"}, ErrDuplicateAdapter, []string{"smtp"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "smtp"}, ErrUnknownAdapter, []string{"smtp"}, &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad host"}, ErrInvalidOptions, []string{"bad host"}, &InvalidOptionsError{}},
		{"address", InvalidAddressError{Field: "from", Value: "bogus"}, ErrInvalidAddress, []string{"from", "bogus"}, &InvalidAddressError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "mailer: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "mailer: ")
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("Error() = %q, want substring %q", msg, sub)
				}
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
