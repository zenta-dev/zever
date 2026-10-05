package redis

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestEdgeNewDefaults checks the applied defaults on a freshly constructed
// adapter: prefix, buffer, and both timeouts.
func TestEdgeNewDefaults(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, nil)

	if a.prefix != "eventbus" {
		t.Errorf("prefix = %q, want %q", a.prefix, "eventbus")
	}

	if a.buffer != eventbus.DefaultBufferSize {
		t.Errorf("buffer = %d, want %d", a.buffer, eventbus.DefaultBufferSize)
	}

	if a.handlerTimeout != eventbus.DefaultHandlerTimeout {
		t.Errorf("handlerTimeout = %v, want %v", a.handlerTimeout, eventbus.DefaultHandlerTimeout)
	}

	if a.closeTimeout != eventbus.DefaultCloseTimeout {
		t.Errorf("closeTimeout = %v, want %v", a.closeTimeout, eventbus.DefaultCloseTimeout)
	}
}

// TestEdgeChannelPrefix checks topic-to-channel mapping with a custom prefix.
func TestEdgeChannelPrefix(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, func(o *eventbus.Options) { o.Redis.Prefix = "pa" })

	if got := a.channel("orders"); got != "pa:orders" {
		t.Fatalf("channel(orders) = %q, want %q", got, "pa:orders")
	}
}

// TestEdgeTopicAtMaxLen checks the inclusive topic-length boundary.
func TestEdgeTopicAtMaxLen(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	ctx := t.Context()
	topic := strings.Repeat("a", eventbus.MaxTopicLen)

	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, topic, func(_ context.Context, m eventbus.Message) { got <- m })
	if err != nil {
		t.Fatalf("Subscribe(at max len) = %v, want nil", err)
	}

	defer unsub()

	if err := b.Publish(ctx, topic, eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish(at max len) = %v, want nil", err)
	}

	waitMsg(t, got)
}

// TestEdgeEmptyPayloadRoundtrip checks that an empty payload with nil
// headers survives the wire envelope.
func TestEdgeEmptyPayloadRoundtrip(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	ctx := t.Context()
	topic := freshTopic()

	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, topic, func(_ context.Context, m eventbus.Message) { got <- m })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	defer unsub()

	if err := b.Publish(ctx, topic, eventbus.Payload{}, nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	m := waitMsg(t, got)
	if len(m.Payload) != 0 {
		t.Fatalf("Payload = %q, want empty", m.Payload)
	}

	if len(m.Headers) != 0 {
		t.Fatalf("Headers = %v, want empty", m.Headers)
	}
}

// TestEdgeDecodeMessageEmptyPayload checks decoding an envelope with no
// payload or headers.
func TestEdgeDecodeMessageEmptyPayload(t *testing.T) {
	t.Parallel()

	const id = "11111111-1111-1111-1111-111111111111"

	raw, err := wireCodec.Encode(wireMessage{ID: id})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	msg, err := decodeMessage("orders", raw)
	if err != nil {
		t.Fatalf("decodeMessage: %v", err)
	}

	if len(msg.Payload) != 0 || len(msg.Headers) != 0 {
		t.Fatalf("decoded = %+v, want empty payload and headers", msg)
	}

	if msg.Topic != "orders" {
		t.Fatalf("Topic = %q, want orders", msg.Topic)
	}
}

// TestEdgeCloseNoSubs checks closing an adapter with no subscriptions is a
// nil-returning, idempotent no-op.
func TestEdgeCloseNoSubs(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestEdgeConcurrentSubscribeUnsubscribe exercises concurrent subscribe and
// unsubscribe on the goroutine-safe adapter.
func TestEdgeConcurrentSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	ctx := t.Context()
	topic := freshTopic()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 10 {
				unsub, err := b.Subscribe(ctx, topic, func(context.Context, eventbus.Message) {})
				if err != nil {
					continue
				}

				unsub()
			}
		}()
	}

	wg.Wait()

	if err := b.Publish(ctx, topic, eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish after churn = %v, want nil", err)
	}
}
