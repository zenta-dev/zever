package cachetest

import (
	"testing"
	"time"
)

// BenchmarkCheckGetSet measures the get-set assertion hot path.
func BenchmarkCheckGetSet(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkGetSet(ctx, healthyStubCache()); err != nil {
			b.Fatalf("checkGetSet() err = %v", err)
		}
	}
}

// BenchmarkCheckCounters measures the counters assertion hot path.
func BenchmarkCheckCounters(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkCounters(ctx, healthyStubCache()); err != nil {
			b.Fatalf("checkCounters() err = %v", err)
		}
	}
}

// BenchmarkCheckDelete measures the delete assertion hot path.
func BenchmarkCheckDelete(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkDelete(ctx, healthyStubCache()); err != nil {
			b.Fatalf("checkDelete() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(ctx, healthyStubCache()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}

// BenchmarkPollExpiry measures the expiry-poll hot path with a short TTL.
func BenchmarkPollExpiry(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stub := healthyStubCache()
		if err := checkTTLExpiry(b.Context(), stub, 2*time.Second); err != nil {
			b.Fatalf("checkTTLExpiry() err = %v", err)
		}
	}
}
