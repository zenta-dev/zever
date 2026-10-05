package memory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/eventbus"
)

var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run so
// Register never collides with a previously registered factory.
func benchFreshAdapter() eventbus.Adapter {
	return eventbus.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

func benchOpts() eventbus.Options {
	return eventbus.Options{
		BufferSize:     1024,
		MaxHandlers:    128,
		HandlerTimeout: time.Second,
		CloseTimeout:   time.Second,
	}
}

// BenchmarkNew measures constructing an in-memory bus.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		mb, err := newBus(benchOpts())
		if err != nil {
			b.Fatalf("newBus() error = %v", err)
		}

		_ = mb.Close()
	}
}

// BenchmarkRegister measures registering a factory into the eventbus registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := eventbus.Register(benchFreshAdapter(), New); err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()
	if err := eventbus.Register(a, New); err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		bus, err := eventbus.Open(a, eventbus.Options{})
		if err != nil {
			b.Fatalf("Open() error = %v", err)
		}

		if err := bus.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

// BenchmarkPublish measures the no-subscriber publish path (message ID,
// header injection, and topic validation).
func BenchmarkPublish(b *testing.B) {
	mb, err := newBus(benchOpts())
	if err != nil {
		b.Fatalf("newBus() error = %v", err)
	}

	b.Cleanup(func() { _ = mb.Close() })

	ctx := b.Context()
	payload := eventbus.NewPayload([]byte("hello"))
	headers := eventbus.NewHeaders(map[string]string{"k": "v"})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := mb.Publish(ctx, "orders", payload, headers); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
}

// BenchmarkPublishFanout measures publish to four subscribers, exercising
// the per-subscriber message clone.
func BenchmarkPublishFanout(b *testing.B) {
	mb, err := newBus(benchOpts())
	if err != nil {
		b.Fatalf("newBus() error = %v", err)
	}

	b.Cleanup(func() { _ = mb.Close() })

	ctx := b.Context()

	for range 4 {
		if _, subErr := mb.Subscribe(ctx, "orders", func(context.Context, eventbus.Message) {}); subErr != nil {
			b.Fatalf("Subscribe() error = %v", subErr)
		}
	}

	payload := eventbus.NewPayload([]byte("hello"))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := mb.Publish(ctx, "orders", payload, nil); err != nil {
			b.Fatalf("Publish() error = %v", err)
		}
	}
}

// BenchmarkSubscribe measures registering and tearing down a handler.
func BenchmarkSubscribe(b *testing.B) {
	mb, err := newBus(benchOpts())
	if err != nil {
		b.Fatalf("newBus() error = %v", err)
	}

	b.Cleanup(func() { _ = mb.Close() })

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		unsub, subErr := mb.Subscribe(ctx, "orders", func(context.Context, eventbus.Message) {})
		if subErr != nil {
			b.Fatalf("Subscribe() error = %v", subErr)
		}

		unsub()
	}
}
