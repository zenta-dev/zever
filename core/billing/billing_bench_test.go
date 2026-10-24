package billing

import (
	"testing"
)

// BenchmarkOptionsValidate measures Options validation for a billing backend.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Endpoint: "https://api.stripe.com/hook"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkParseMinorUnits measures the decimal-to-minor-units parse hot path.
func BenchmarkParseMinorUnits(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ParseMinorUnits("10.00", "usd"); err != nil {
			b.Fatalf("ParseMinorUnits() err = %v", err)
		}
	}
}

// BenchmarkCurrencyExponent measures the currency exponent lookup hot path.
func BenchmarkCurrencyExponent(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if CurrencyExponent("usd") != 2 {
			b.Fatal("CurrencyExponent(usd) != 2")
		}
	}
}
