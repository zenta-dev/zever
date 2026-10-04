package billing

import (
	"errors"
	"testing"
)

func TestEdgeParseMinorUnits_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		currency string
		want     int64
	}{
		{"zero", "0", "usd", 0},
		{"negative half up", "-10.005", "usd", -1001},
		{"negative half down", "-10.004", "usd", -1000},
		{"empty currency defaults two", "1.23", "", 123},
		{"upper currency", "1.23", "USD", 123},
		{"exact max fits", "92233720368547757.99", "usd", 9223372036854775799},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinorUnits(tc.input, tc.currency)
			if err != nil {
				t.Fatalf("ParseMinorUnits(%q, %q) err = %v", tc.input, tc.currency, err)
			}

			if got != tc.want {
				t.Fatalf("ParseMinorUnits(%q, %q) = %d, want %d", tc.input, tc.currency, got, tc.want)
			}
		})
	}
}

func TestEdgeParseMinorUnits_whitespaceInside(t *testing.T) {
	t.Parallel()

	if _, err := ParseMinorUnits("1 2", "usd"); !errors.Is(err, ErrMalformedAmount) {
		t.Fatalf("err = %v, want ErrMalformedAmount", err)
	}
}

func TestEdgeParseMinorUnits_overflowFraction(t *testing.T) {
	t.Parallel()

	if _, err := ParseMinorUnits("92233720368547758.07", "usd"); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("err = %v, want ErrAmountOverflow", err)
	}
}
