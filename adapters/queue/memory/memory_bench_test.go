package memory

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

// newBenchQueue opens a memory queue with a generous poll timeout so a
// transiently empty Pop waits for the next Push instead of failing the run.
func newBenchQueue(b *testing.B) queue.Queue {
	b.Helper()

	q, err := New(queue.Options{
		PollTimeout:       5 * time.Second,
		VisibilityTimeout: time.Minute,
	})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}
	b.Cleanup(func() { _ = q.Close() })

	return q
}

// roundTrip pushes one message, pops it back, and acks it. Push and Pop stay
// balanced per iteration, so the queue never grows and the memory footprint
// stays flat regardless of benchtime.
func roundTrip(ctx context.Context, b *testing.B, q queue.Queue, topic string, payload queue.Payload) {
	b.Helper()

	if err := q.Push(ctx, topic, payload, nil); err != nil {
		b.Fatalf("Push(): %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		b.Fatalf("Pop(): %v", err)
	}

	if err := q.Ack(ctx, msg); err != nil {
		b.Fatalf("Ack(): %v", err)
	}
}

// BenchmarkMemoryRoundTrip measures a sequential Push/Pop/Ack cycle on the
// memory adapter -- the no-contention baseline for the parallel bench below.
func BenchmarkMemoryRoundTrip(b *testing.B) {
	q := newBenchQueue(b)
	ctx := b.Context()
	payload := queue.Payload([]byte("bench-payload"))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		roundTrip(ctx, b, q, "bench", payload)
	}
}

// BenchmarkMemoryRoundTripParallel measures Push/Pop/Ack throughput under
// concurrent load on one shared topic. Each iteration stays balanced
// (one push, one pop, one ack), so workers contend on the topic locks
// without the queue growing.
func BenchmarkMemoryRoundTripParallel(b *testing.B) {
	q := newBenchQueue(b)
	ctx := b.Context()
	payload := queue.Payload([]byte("bench-payload"))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := q.Push(ctx, "bench", payload, nil); err != nil {
				b.Error(err)
				return
			}

			msg, err := q.Pop(ctx, "bench")
			if err != nil {
				b.Error(err)
				return
			}

			if err := q.Ack(ctx, msg); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
