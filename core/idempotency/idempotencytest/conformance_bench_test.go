package idempotencytest

import (
	"testing"
)

// BenchmarkCheckExecuteReplay measures the execute-replay assertion hot path.
func BenchmarkCheckExecuteReplay(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkExecuteReplay(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkExecuteReplay() err = %v", err)
		}
	}
}

// BenchmarkCheckFingerprintMismatch measures the mismatch assertion hot path.
func BenchmarkCheckFingerprintMismatch(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkFingerprintMismatch(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkFingerprintMismatch() err = %v", err)
		}
	}
}

// BenchmarkCheckForget measures the forget assertion hot path.
func BenchmarkCheckForget(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkForget(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkForget() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}
