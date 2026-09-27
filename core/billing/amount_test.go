package billing

import (
	"errors"
	"testing"
)

func TestParseMinorUnits_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		currency string
		want     int64
	}{
		{"dollars cents", "10.00", "usd", 1000},
		{"whole", "10", "usd", 1000},
		{"cent", "0.01", "usd", 1},
		{"half up", "10.005", "usd", 1001},
		{"half down", "10.004", "usd", 1000},
		{"round", "10.567", "usd", 1057},
		{"jpy whole", "100", "jpy", 100},
		{"jpy fraction rounds", "100.5", "jpy", 101},
		{"kwd three decimals", "1.234", "kwd", 1234},
		{"clf four decimals", "1.2345", "clf", 12345},
		{"trim spaces", "  10.00  ", "usd", 1000},
		{"explicit plus", "+5", "usd", 500},
		{"negative", "-3.25", "usd", -325},
		{"long fraction truncates", "10.123456", "usd", 1012},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinorUnits(tc.input, tc.currency)
			if err != nil {
				t.Fatalf("ParseMinorUnits(%q, %q) err = %v", tc.input, tc.currency, err)
			}

			if got != tc.want {
				t.Errorf("ParseMinorUnits(%q, %q) = %d, want %d", tc.input, tc.currency, got, tc.want)
			}
		})
	}
}

func TestParseMinorUnits_errors_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		currency string
		is       error
	}{
		{"empty", "", "usd", ErrEmptyAmount},
		{"spaces only", "   ", "usd", ErrEmptyAmount},
		{"lone minus", "-", "usd", ErrMalformedAmount},
		{"lone plus", "+", "usd", ErrMalformedAmount},
		{"leading dot", ".5", "usd", ErrMalformedAmount},
		{"trailing dot", "5.", "usd", ErrMalformedAmount},
		{"alpha", "abc", "usd", ErrMalformedAmount},
		{"double dot", "1.2.3", "usd", ErrMalformedAmount},
		{"overflow whole", "92233720368547758", "usd", ErrAmountOverflow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinorUnits(tc.input, tc.currency)
			if !errors.Is(err, tc.is) {
				t.Fatalf("ParseMinorUnits(%q) err = %v, want %v", tc.input, err, tc.is)
			}

			if got != 0 {
				t.Errorf("ParseMinorUnits(%q) = %d, want 0 on error", tc.input, got)
			}
		})
	}
}
