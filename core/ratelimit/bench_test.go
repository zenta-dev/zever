package ratelimit

import (
	"testing"
)

// BenchmarkValidateKey measures rate-limit key validation.
func BenchmarkValidateKey(b *testing.B) {
	key := "user:123:endpoint"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateKey(key); err != nil {
			b.Fatalf("ValidateKey(%q) error = %v", key, err)
		}
	}
}

func BenchmarkValidateCost(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateCost(1, 100); err != nil {
			b.Fatalf("ValidateCost error = %v", err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Rate: 100, Burst: 200, IdleTTL: DefaultIdleTTL, SweepInterval: DefaultSweepInterval}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
