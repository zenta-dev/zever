package redis

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/eventbus"
)

var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run so
// Register never collides with a previously registered factory.
func benchFreshAdapter() eventbus.Adapter {
	return eventbus.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

// benchPusher is a no-op Pusher used to isolate registry benchmarks from a
// live Redis broker.
type benchPusher struct{}

func (benchPusher) Publish(context.Context, string, eventbus.Payload, eventbus.Headers) error {
	return nil
}

func (benchPusher) Subscribe(context.Context, string, eventbus.Handler) (func(), error) {
	return func() {}, nil
}

func (benchPusher) Close() error { return nil }

func (benchPusher) Name() string { return "bench" }

// benchRedisAdapter builds a Redis bus over a per-benchmark miniredis.
func benchRedisAdapter(b *testing.B) eventbus.EventBus {
	b.Helper()

	s := miniredis.RunT(b)

	bus, err := New(eventbus.Options{Redis: eventbus.RedisOptions{Addr: s.Addr()}})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = bus.Close() })

	return bus
}

// BenchmarkNew measures constructing a Redis-backed bus (connect + ping).
func BenchmarkNew(b *testing.B) {
	s := miniredis.RunT(b)
	opts := eventbus.Options{Redis: eventbus.RedisOptions{Addr: s.Addr()}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		bus, err := New(opts)
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := bus.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkRegister measures registering a factory into the eventbus registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		err := eventbus.Register(benchFreshAdapter(), func(eventbus.Options) (eventbus.EventBus, error) {
			return eventbus.Wrap(benchPusher{}), nil
		})
		if err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus wrapped construction via Open.
func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()

	err := eventbus.Register(a, func(eventbus.Options) (eventbus.EventBus, error) {
		return eventbus.Wrap(benchPusher{}), nil
	})
	if err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		bus, openErr := eventbus.Open(a, eventbus.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := bus.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}

// BenchmarkPublish measures the encode-and-publish hot path with no
// subscribers (at-most-once silent drop).
func BenchmarkPublish(b *testing.B) {
	bus := benchRedisAdapter(b)
	ctx := b.Context()
	payload := eventbus.NewPayload([]byte("hello"))
	headers := eventbus.NewHeaders(map[string]string{"k": "v"})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := bus.Publish(ctx, "orders", payload, headers); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
}

// BenchmarkSubscribe measures the subscribe handshake plus teardown.
func BenchmarkSubscribe(b *testing.B) {
	bus := benchRedisAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		unsub, err := bus.Subscribe(ctx, "orders", func(context.Context, eventbus.Message) {})
		if err != nil {
			b.Fatalf("Subscribe() error = %v", err)
		}

		unsub()
	}
}

// BenchmarkDecodeMessage measures wire-envelope decoding.
func BenchmarkDecodeMessage(b *testing.B) {
	const id = "11111111-1111-1111-1111-111111111111"

	raw, err := wireCodec.Encode(wireMessage{
		ID:      id,
		Payload: []byte("hello"),
		Headers: map[string]string{"k": "v"},
	})
	if err != nil {
		b.Fatalf("Encode() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, decodeErr := decodeMessage("orders", raw); decodeErr != nil {
			b.Fatalf("decodeMessage() error = %v", decodeErr)
		}
	}
}
