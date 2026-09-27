package flag

import (
	"strings"
	"testing"
)

// TestCoverTypedErrorStrings exercises every typed Error method so the
// message constructors stay covered. It asserts prefixes, not exact
// text, per the no-brittle-assertions rule.
func TestCoverTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"duplicate", DuplicateError{Adapter: Static}, "flag: duplicate registration"},
		{"unknown", UnknownAdapterError{Adapter: Adapter("")}, "flag: unknown adapter"},
		{"invalidAdapter", InvalidAdapterError{Adapter: "bogus"}, "flag: invalid adapter"},
		{"invalidOptions", InvalidOptionsError{Reason: "bad"}, "flag: invalid options"},
		{"invalidKey", InvalidKeyError{KeyLen: 300}, "flag: invalid key"},
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
