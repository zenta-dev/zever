package flagtest

import (
	"testing"
)

// BenchmarkCheckFallback measures the fallback assertion hot path.
func BenchmarkCheckFallback(b *testing.B) {
	ctx := b.Context()
	stub := healthyStubFlag()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkFallback(ctx, stub); err != nil {
			b.Fatalf("checkFallback() err = %v", err)
		}
	}
}

// BenchmarkCheckInvalidKey measures the invalid-key assertion hot path.
func BenchmarkCheckInvalidKey(b *testing.B) {
	ctx := b.Context()
	stub := healthyStubFlag()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkInvalidKey(ctx, stub); err != nil {
			b.Fatalf("checkInvalidKey() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(healthyStubFlag()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}
