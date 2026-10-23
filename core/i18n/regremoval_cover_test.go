package i18n

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
		{"duplicate", DuplicateAdapterError{Adapter: "po"}, ErrDuplicateAdapter, []string{"po"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "po"}, ErrUnknownAdapter, []string{"po"}, &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad dir"}, ErrInvalidOptions, []string{"bad dir"}, &InvalidOptionsError{}},
		{"locale", LocaleNotFoundError{Locale: "xx"}, ErrLocaleNotFound, []string{"xx"}, &LocaleNotFoundError{}},
		{"key", KeyNotFoundError{Locale: "en", Key: "missing"}, ErrKeyNotFound, []string{"en", "missing"}, &KeyNotFoundError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "i18n: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "i18n: ")
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
