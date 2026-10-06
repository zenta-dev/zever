package sessiontest

import (
	"context"
	"testing"
	"time"
)

// BenchmarkCheckCreateGet measures the create-get assertion hot path.
func BenchmarkCheckCreateGet(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkCreateGet(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkCreateGet() err = %v", err)
		}
	}
}

// BenchmarkCheckSave measures the save assertion hot path.
func BenchmarkCheckSave(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkSave(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkSave() err = %v", err)
		}
	}
}

// BenchmarkCheckDelete measures the delete assertion hot path.
func BenchmarkCheckDelete(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkDelete(ctx, healthyStubStore()); err != nil {
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
		if err := checkClose(ctx, healthyStubStore()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}

// BenchmarkPollExpiry measures the expiry-poll hot path with an
// immediately-true condition.
func BenchmarkPollExpiry(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := pollExpiry(ctx, time.Second, "bench", func(context.Context) bool {
			return true
		}); err != nil {
			b.Fatalf("pollExpiry() err = %v", err)
		}
	}
}
