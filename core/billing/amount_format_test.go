package billing

import (
	"math"
	"testing"
)

func TestFormatMinorUnits_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		value    int64
		currency string
		want     string
	}{
		{"zero usd", 0, "usd", "0.00"},
		{"dollars", 1000, "usd", "10.00"},
		{"cent", 1, "usd", "0.01"},
		{"negative", -325, "usd", "-3.25"},
		{"negative cent", -1, "usd", "-0.01"},
		{"jpy whole", 100, "jpy", "100"},
		{"jpy negative", -5, "jpy", "-5"},
		{"kwd three decimals", 1234, "kwd", "1.234"},
		{"clf four decimals", 12345, "clf", "1.2345"},
		{"max int64 usd", math.MaxInt64, "usd", "92233720368547758.07"},
		{"min int64 usd", math.MinInt64, "usd", "-92233720368547758.08"},
		{"max int64 jpy", math.MaxInt64, "jpy", "9223372036854775807"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := FormatMinorUnits(tc.value, tc.currency); got != tc.want {
				t.Errorf("FormatMinorUnits(%d, %q) = %q, want %q", tc.value, tc.currency, got, tc.want)
			}
		})
	}
}

func TestMinorUnitsRoundTrip(t *testing.T) {
	t.Parallel()

	values := []int64{0, 1, -1, 100, -100, 1000, -1001, 1057, math.MaxInt64, math.MinInt64, math.MaxInt64 - 1}
	currencies := []string{"usd", "jpy", "kwd", "clf", "bhd"}
	for _, currency := range currencies {
		for _, v := range values {
			// math.MinInt64 has no positive int64 literal, so zero-exponent
			// currencies cannot write its whole part; every other value
			// round-trips.
			if v == math.MinInt64 && CurrencyExponent(currency) == 0 {
				continue
			}

			got, err := ParseMinorUnits(FormatMinorUnits(v, currency), currency)
			if err != nil {
				t.Errorf("ParseMinorUnits(FormatMinorUnits(%d, %q)) err = %v", v, currency, err)
				continue
			}

			if got != v {
				t.Errorf("round-trip(%d, %q) = %d", v, currency, got)
			}
		}
	}
}

func TestParseMinorUnits_negativeRounding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  int64
	}{
		{"-10.005", -1001},
		{"-10.004", -1000},
		{"-0.005", -1},
		{"-0.004", 0},
		{"-99.999", -10000},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinorUnits(tc.input, "usd")
			if err != nil {
				t.Fatalf("ParseMinorUnits(%q) err = %v", tc.input, err)
			}

			if got != tc.want {
				t.Errorf("ParseMinorUnits(%q) = %d, want %d (half away from zero)", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseMinorUnits_exactInt64Boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		currency string
		want     int64
	}{
		{"max usd", "92233720368547758.07", "usd", math.MaxInt64},
		{"min usd", "-92233720368547758.08", "usd", math.MinInt64},
		{"max jpy", "9223372036854775807", "jpy", math.MaxInt64},
		{"max minus one jpy", "9223372036854775806", "jpy", math.MaxInt64 - 1},
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

	for _, tc := range []struct{ input, currency string }{
		{"92233720368547758.08", "usd"},
		{"-92233720368547758.09", "usd"},
		{"9223372036854775807.5", "jpy"},
		{"92233720368547759", "usd"},
	} {
		if _, err := ParseMinorUnits(tc.input, tc.currency); err == nil {
			t.Errorf("ParseMinorUnits(%q, %q) err = nil, want overflow", tc.input, tc.currency)
		}
	}
}
