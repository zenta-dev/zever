package billing

import "testing"

func TestCurrencyExponent_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		currency string
		want     int
	}{
		{"clf four", "clf", 4},
		{"clf upper", "CLF", 4},
		{"kwd three", "kwd", 3},
		{"bhd three", "bhd", 3},
		{"jpy zero", "jpy", 0},
		{"jpy upper", "JPY", 0},
		{"krw zero", "krw", 0},
		{"usd two", "usd", 2},
		{"usd upper", "USD", 2},
		{"unknown two", "zzz", 2},
		{"empty two", "", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CurrencyExponent(tc.currency); got != tc.want {
				t.Errorf("CurrencyExponent(%q) = %d, want %d", tc.currency, got, tc.want)
			}
		})
	}
}
