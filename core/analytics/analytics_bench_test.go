package analytics

import (
	"testing"
)

// benchAdapter registers a stub analytics backend once and returns its
// adapter so Open can be measured without paying the registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Analytics, error) { return &stubAnalytics{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(Options) (Analytics, error) { return &stubAnalytics{}, nil }

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
	opts := Options{
		Endpoint:           "https://example.com/track",
		MaxProperties:      100,
		MaxPropertiesBytes: 1024,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkValidatePropertiesSize measures the JSON size guard hot path.
func BenchmarkValidatePropertiesSize(b *testing.B) {
	m := map[string]any{"k": "v", "n": 1, "b": true}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidatePropertiesSize(m, 1024); err != nil {
			b.Fatalf("ValidatePropertiesSize() err = %v", err)
		}
	}
}

// BenchmarkMarshalValidated measures the marshal-plus-size-check hot path.
func BenchmarkMarshalValidated(b *testing.B) {
	m := map[string]any{"k": "v", "n": 1, "b": true}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := MarshalValidated(m, 1024); err != nil {
			b.Fatalf("MarshalValidated() err = %v", err)
		}
	}
}

// BenchmarkValidateBounds measures the count-then-size guard hot path.
func BenchmarkValidateBounds(b *testing.B) {
	m := map[string]any{"k": "v", "n": 1, "b": true}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateBounds(m, 100, 1024); err != nil {
			b.Fatalf("ValidateBounds() err = %v", err)
		}
	}
}

// BenchmarkUserIDContext measures the context value round-trip hot path.
func BenchmarkUserIDContext(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := UserID(WithUserID(ctx, "u1")); got != "u1" {
			b.Fatal("UserID roundtrip failed")
		}
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ParseAdapter("posthog"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
		}
	}
}
