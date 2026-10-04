package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/eventbus"
)

// benchBus opens a redis-backed bus over a throwaway miniredis instance.
func benchBus(b *testing.B) eventbus.EventBus {
	b.Helper()

	s, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis Run: %v", err)
	}

	b.Cleanup(s.Close)

	eb, err := New(eventbus.Options{
		Redis:          eventbus.RedisOptions{Addr: s.Addr()},
		BufferSize:     eventbus.DefaultBufferSize,
		HandlerTimeout: eventbus.DefaultHandlerTimeout,
		CloseTimeout:   eventbus.DefaultCloseTimeout,
	})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = eb.Close() })

	return eb
}

// BenchmarkPublish measures the encode-and-PUBLISH round trip to miniredis.
func BenchmarkPublish(b *testing.B) {
	eb := benchBus(b)
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

// BenchmarkPublishParallel measures concurrent PUBLISH round trips.
func BenchmarkPublishParallel(b *testing.B) {
	eb := benchBus(b)
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
