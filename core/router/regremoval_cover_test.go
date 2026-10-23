package router

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise the adapter
// errors via duplicate/unknown/invalid paths, so direct unit tests now pin
// the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSubs []string
		asTarget any
	}{
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"duplicate", DuplicateAdapterError{Adapter: "mux"}, ErrDuplicateAdapter, []string{"mux"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "mux"}, ErrUnknownAdapter, []string{"mux"}, &UnknownAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad base"}, ErrInvalidOptions, []string{"bad base"}, &InvalidOptionsError{}},
		{"method", InvalidMethodError{Method: "BREW"}, ErrInvalidMethod, []string{"BREW"}, &InvalidMethodError{}},
		{
			"pattern",
			MalformedPatternError{Pattern: "/x/{", Reason: "unclosed"},
			ErrMalformedPattern,
			[]string{"/x/{", "unclosed"},
			&MalformedPatternError{},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "router: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "router: ")
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
