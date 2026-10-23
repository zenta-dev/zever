package analytics

import (
	"testing"
)

// benchAdapter registers a stub analytics backend once and returns its
// adapter so Open can be measured without paying the registration cost.
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
