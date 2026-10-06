package ratelimittest

import (
	"testing"
)

// BenchmarkCheckAllowDeny measures the allow/deny assertion hot path.
func BenchmarkCheckAllowDeny(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkAllowDeny(ctx, healthyStubLimiter()); err != nil {
			b.Fatalf("checkAllowDeny() err = %v", err)
		}
	}
}

// BenchmarkCheckReset measures the reset assertion hot path.
func BenchmarkCheckReset(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkReset(ctx, healthyStubLimiter()); err != nil {
			b.Fatalf("checkReset() err = %v", err)
		}
	}
}

// BenchmarkCheckInvalidInput measures the invalid-input assertion hot path.
func BenchmarkCheckInvalidInput(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkInvalidInput(ctx, healthyStubLimiter()); err != nil {
			b.Fatalf("checkInvalidInput() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(ctx, healthyStubLimiter()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}
