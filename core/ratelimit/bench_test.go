package ratelimit

import (
	"testing"
)

// benchAdapter registers a stub limiter once and returns its adapter so Open
// can be measured without paying the one-shot registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Limiter, error) { return stubLimiter{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)
	opts := Options{Rate: 100, Burst: 200}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)
	opts := Options{Rate: 100, Burst: 200}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

func BenchmarkValidateKey(b *testing.B) {
	key := "user:123:endpoint"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ValidateKey(key); err != nil {
			b.Fatalf("ValidateKey(%q) error = %v", key, err)
		}
	}
}

func BenchmarkValidateCost(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ValidateCost(1, 100); err != nil {
			b.Fatalf("ValidateCost error = %v", err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Rate: 100, Burst: 200, IdleTTL: DefaultIdleTTL, SweepInterval: DefaultSweepInterval}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
