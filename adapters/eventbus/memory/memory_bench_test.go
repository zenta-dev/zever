package memory

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// benchBus opens an in-memory bus with subs no-op subscribers on "bench".
func benchBus(b *testing.B, subs int) eventbus.EventBus {
	b.Helper()

	eb, err := New(eventbus.Options{
		BufferSize:     eventbus.DefaultBufferSize,
		MaxHandlers:    eventbus.DefaultMaxHandlers,
		HandlerTimeout: eventbus.DefaultHandlerTimeout,
		CloseTimeout:   eventbus.DefaultCloseTimeout,
	})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = eb.Close() })

	for range subs {
		if _, err := eb.Subscribe(b.Context(), "bench", func(context.Context, eventbus.Message) {}); err != nil {
			b.Fatalf("Subscribe: %v", err)
		}
	}

	return eb
}

// BenchmarkPublish measures the push hot path: topic validation, message
// construction, and fanout enqueue to one subscriber.
func BenchmarkPublish(b *testing.B) {
	eb := benchBus(b, 1)
	ctx := b.Context()
	payload := eventbus.NewPayload([]byte("hello"))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := eb.Publish(ctx, "bench", payload, nil); err != nil {
			b.Fatalf("Publish: %v", err)
		}
	}
}

// BenchmarkPublishParallel measures concurrent fanout to four subscribers.
func BenchmarkPublishParallel(b *testing.B) {
	eb := benchBus(b, 4)
	ctx := b.Context()
	payload := eventbus.NewPayload([]byte("hello"))

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := eb.Publish(ctx, "bench", payload, nil); err != nil {
				b.Fatalf("Publish: %v", err)
			}
		}
	})
}
