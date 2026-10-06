package queuetest

import (
	"testing"
	"time"
)

// BenchmarkCheckFIFO measures the FIFO assertion hot path.
func BenchmarkCheckFIFO(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkFIFO(ctx, healthyStubQueue()); err != nil {
			b.Fatalf("checkFIFO() err = %v", err)
		}
	}
}

// BenchmarkCheckAck measures the ack assertion hot path.
func BenchmarkCheckAck(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkAck(ctx, healthyStubQueue()); err != nil {
			b.Fatalf("checkAck() err = %v", err)
		}
	}
}

// BenchmarkCheckNackDrop measures the nack-drop assertion hot path.
func BenchmarkCheckNackDrop(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkNackDrop(ctx, healthyStubQueue()); err != nil {
			b.Fatalf("checkNackDrop() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(ctx, healthyStubQueue()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}

// BenchmarkCheckDelayed measures the delayed-delivery assertion hot path.
func BenchmarkCheckDelayed(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stub := healthyStubQueue()
		if err := checkDelayed(b.Context(), stub, time.Millisecond, 2*time.Second); err != nil {
			b.Fatalf("checkDelayed() err = %v", err)
		}
	}
}
