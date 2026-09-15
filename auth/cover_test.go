package auth

import (
	"strings"
	"testing"
)

// TestCoverTypedErrorStrings exercises the registry Error methods so
// message constructors stay covered. Prefix asserts only, per the
// no-brittle-assertions rule.
func TestCoverTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"duplicate", DuplicateError{Adapter: JWT}, "auth: duplicate registration"},
		{"unknown", UnknownAdapterError{Adapter: Adapter(99)}, "auth: unknown adapter"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.err.Error(); !strings.HasPrefix(got, tc.want) {
				t.Errorf("Error() = %q, want prefix %q", got, tc.want)
			}
		})
	}
}
