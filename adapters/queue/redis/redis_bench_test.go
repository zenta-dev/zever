package redis

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/queue"
)

func newBenchQueue(b *testing.B) queue.Queue {
	b.Helper()
	s := miniredis.RunT(b)
	q, err := New(queue.Options{Addr: s.Addr(), PollTimeout: time.Second, VisibilityTimeout: time.Minute})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = q.Close() })
	return q
}

func benchRoundTrip(b *testing.B, q queue.Queue, topic string) {
	b.Helper()
	ctx := b.Context()
	payload := queue.Payload([]byte("bench-payload"))
	if err := q.Push(ctx, topic, payload, nil); err != nil {
		b.Fatalf("Push() = %v", err)
	}
	msg, err := q.Pop(ctx, topic)
	if err != nil {
		b.Fatalf("Pop() = %v", err)
	}
	if err := q.Ack(ctx, msg); err != nil {
		b.Fatalf("Ack() = %v", err)
	}
}

// BenchmarkRoundTrip measures a sequential Push/Pop/Ack cycle against
// in-process miniredis.
func BenchmarkRoundTrip(b *testing.B) {
	q := newBenchQueue(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchRoundTrip(b, q, "bench")
	}
}

// BenchmarkRoundTripParallel measures concurrent Push/Pop/Ack cycles. Each
// worker gets its own topic so an empty Pop never blocks behind another
// worker's message.
func BenchmarkRoundTripParallel(b *testing.B) {
	q := newBenchQueue(b)
	var seq atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		topic := "bench-" + strconv.FormatInt(seq.Add(1), 10)
		for pb.Next() {
			benchRoundTrip(b, q, topic)
		}
	})
}
