package outboxbridge

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/queue"
)

// sinkQueue discards pushes; the embedded nil interface panics on any other
// method, which the publisher never calls.
type sinkQueue struct{ queue.Queue }

func (sinkQueue) Push(context.Context, string, queue.Payload, queue.Headers) error { return nil }

type sinkBus struct{ eventbus.EventBus }

func (sinkBus) Publish(context.Context, string, eventbus.Payload, eventbus.Headers) error {
	return nil
}

func benchPublish(b *testing.B, p outbox.Publisher, msg outbox.Message) {
	b.Helper()

	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		if err := p.Publish(ctx, msg); err != nil {
			b.Fatalf("Publish: %v", err)
		}
	}
}

func BenchmarkQueuePublisher(b *testing.B) {
	p, err := QueuePublisher(sinkQueue{})
	if err != nil {
		b.Fatal(err)
	}

	benchPublish(b, p, sampleMessage())
}

func BenchmarkQueuePublisherNoHeaders(b *testing.B) {
	p, err := QueuePublisher(sinkQueue{})
	if err != nil {
		b.Fatal(err)
	}

	benchPublish(b, p, outbox.Message{ID: "m", Topic: "t", Payload: []byte("x")})
}

func BenchmarkEventBusPublisher(b *testing.B) {
	p, err := EventBusPublisher(sinkBus{})
	if err != nil {
		b.Fatal(err)
	}

	benchPublish(b, p, sampleMessage())
}
