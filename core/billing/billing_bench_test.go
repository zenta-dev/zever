package billing

import (
	"testing"
)

// benchAdapter registers a stub billing backend once and returns its adapter
// so Open can be measured without paying the registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Billing, error) { return &stubBilling{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(Options) (Billing, error) { return &stubBilling{}, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a := freshAdapter()
		if err := Register(a, factory); err != nil {
			b.Fatalf("Register(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpen measures the registry lookup plus factory invocation hot path.
func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpenParallel measures concurrent Open calls on one adapter.
func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Errorf("Open(%v) err = %v", a, err)
				return
			}
		}
	})
}

// BenchmarkOptionsValidate measures options validation with an endpoint present.
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

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ParseAdapter("stripe"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
		}
	}
}
