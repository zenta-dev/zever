package firebase

import "testing"

// TestEdgeParseFirebaseInt covers boundary and malformed integer encodings:
// whitespace, sign, exponent form, truncation, and overflow.
func TestEdgeParseFirebaseInt(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		in   string
		want int
		ok   bool
	}{
		"empty":    {"", 0, false},
		"space":    {" 7 ", 7, true},
		"plus":     {"+5", 5, true},
		"negative": {"-4", -4, true},
		"exp":      {"1e3", 1000, true},
		"frac":     {"3.9", 3, true},
		"nan":      {"NaN", 0, false},
		"inf":      {"Inf", 0, false},
		"overflow": {"99999999999999999999", 0, false},
		"garbage":  {"abc", 0, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseFirebaseInt(tc.in)

			switch {
			case tc.ok && err != nil:
				t.Fatalf("parseFirebaseInt(%q) err = %v, want nil", tc.in, err)
			case !tc.ok && err == nil:
				t.Fatalf("parseFirebaseInt(%q) err = nil, want non-nil", tc.in)
			case tc.ok && got != tc.want:
				t.Fatalf("parseFirebaseInt(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
