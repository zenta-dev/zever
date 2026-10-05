package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestEdgePublishPayloadBoundary checks that a payload exactly at
// MaxMessageSize is accepted while one byte over is rejected.
func TestEdgePublishPayloadBoundary(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer b.Close()

	ctx := t.Context()

	atLimit := make(eventbus.Payload, eventbus.MaxMessageSize)
	if err := b.Publish(ctx, "edge", atLimit, nil); err != nil {
		t.Fatalf("Publish(at limit) = %v, want nil", err)
	}

	overLimit := make(eventbus.Payload, eventbus.MaxMessageSize+1)
	if err := b.Publish(ctx, "edge", overLimit, nil); !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Fatalf("Publish(over limit) = %v, want ErrPayloadTooLarge", err)
	}
}

// TestEdgeTopicAtMaxLen checks the inclusive topic-length boundary.
func TestEdgeTopicAtMaxLen(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer b.Close()

	ctx := t.Context()
	topic := strings.Repeat("a", eventbus.MaxTopicLen)

	got := make(chan eventbus.Message, 1)

	if _, subErr := b.Subscribe(ctx, topic, func(_ context.Context, m eventbus.Message) { got <- m }); subErr != nil {
		t.Fatalf("Subscribe(at max len) = %v, want nil", subErr)
	}

	if pubErr := b.Publish(ctx, topic, eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
		t.Fatalf("Publish(at max len) = %v, want nil", pubErr)
	}

	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for max-length topic delivery")
	}
}

// TestEdgeEmptyPayloadNilHeaders checks that an empty payload with nil
// headers round-trips unchanged.
func TestEdgeEmptyPayloadNilHeaders(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer b.Close()

	ctx := t.Context()
	got := make(chan eventbus.Message, 1)

	if _, subErr := b.Subscribe(ctx, "empty", func(_ context.Context, m eventbus.Message) { got <- m }); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := b.Publish(ctx, "empty", eventbus.Payload{}, nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	m := recvTimeout(t, got, 2*time.Second)
	if len(m.Payload) != 0 {
		t.Fatalf("Payload = %q, want empty", m.Payload)
	}

	if m.Headers != nil {
		t.Fatalf("Headers = %v, want nil", m.Headers)
	}
}

// TestEdgeNoSubscribersPublish checks that publishing to a topic with no
// subscribers is a silent no-op.
func TestEdgeNoSubscribersPublish(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer b.Close()

	if err := b.Publish(t.Context(), "orphan", eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish(no subscribers) = %v, want nil", err)
	}
}

// TestEdgeCloseNoSubs checks that closing a bus with no subscriptions is a
// nil-returning, idempotent no-op.
func TestEdgeCloseNoSubs(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestEdgeDroppedStartsZero checks the dropped counter starts at zero.
func TestEdgeDroppedStartsZero(t *testing.T) {
	t.Parallel()

	mb := coverMustBus(t, testOpts())
	defer mb.Close()

	unsub, err := mb.Subscribe(t.Context(), "d", func(context.Context, eventbus.Message) {})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	defer unsub()

	mb.mu.RLock()
	subs := mb.topics["d"]
	mb.mu.RUnlock()

	if len(subs) != 1 {
		t.Fatalf("got %d subs want 1", len(subs))
	}

	if got := subs[0].Dropped(); got != 0 {
		t.Fatalf("Dropped() = %d, want 0", got)
	}
}

// TestEdgeConcurrentPublishSubscribe exercises concurrent publish and
// subscribe/unsubscribe on a goroutine-safe bus.
func TestEdgeConcurrentPublishSubscribe(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer b.Close()

	ctx := t.Context()
	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 50 {
				_ = b.Publish(ctx, "race", eventbus.NewPayload([]byte("x")), nil)
			}
		}()
	}

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 25 {
				unsub, subErr := b.Subscribe(ctx, "race", func(context.Context, eventbus.Message) {})
				if subErr != nil {
					continue
				}

				unsub()
			}
		}()
	}

	wg.Wait()
}
