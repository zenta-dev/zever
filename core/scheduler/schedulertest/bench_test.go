package schedulertest

import (
	"testing"

	"github.com/zenta-dev/zever/core/queue"
)

// BenchmarkStubQueuePush measures stub queue push throughput.
func BenchmarkStubQueuePush(b *testing.B) {
	s := newStubQueue()
	ctx := b.Context()
	payload := queue.Payload("bench")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Push(ctx, "bench", payload, nil); err != nil {
			b.Fatalf("Push error = %v", err)
		}
	}
}

// BenchmarkStubQueuePop measures stub queue pop throughput.
func BenchmarkStubQueuePop(b *testing.B) {
	s := newStubQueue()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Push(ctx, "bench", queue.Payload("bench"), nil); err != nil {
			b.Fatalf("Push error = %v", err)
		}

		if _, err := s.Pop(ctx, "bench"); err != nil {
			b.Fatalf("Pop error = %v", err)
		}
	}
}
