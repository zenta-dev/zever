package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeBus is a synchronous in-process Pusher for pull-wrapper tests.
// It must stay stdlib-only: adapter packages cannot be imported here
// without creating an import cycle.
type fakeBus struct {
	mu       sync.Mutex
	subs     map[string][]Handler
	unsubbed int
	closed   bool
}

func newFakeBus() *fakeBus {
	return &fakeBus{subs: make(map[string][]Handler)}
}

func (b *fakeBus) Publish(ctx context.Context, topic string, payload Payload, headers Headers) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrClosed
	}

	msg := NewMessage(topic, payload, headers)
	for _, h := range b.subs[topic] {
		h(ctx, msg)
	}

	return nil
}

func (b *fakeBus) Subscribe(_ context.Context, topic string, handler Handler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrClosed
	}

	b.subs[topic] = append(b.subs[topic], handler)
	idx := len(b.subs[topic]) - 1

	var once sync.Once

	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			handlers := b.subs[topic]
			if idx < len(handlers) {
				b.subs[topic] = append(handlers[:idx:idx], handlers[idx+1:]...)
				b.unsubbed++
			}
		})
	}, nil
}

func (b *fakeBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true

	return nil
}

func (b *fakeBus) Name() string { return "fake" }

func recvChan(t *testing.T, ch <-chan Message) Message {
	t.Helper()

	select {
	case m := <-ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
		return Message{}
	}
}

func TestNewMessage_stampsReceivedAt(t *testing.T) {
	t.Parallel()

	before := time.Now()
	m := NewMessage("orders", NewPayload([]byte("hi")), nil)
	after := time.Now()

	if m.ReceivedAt.IsZero() {
		t.Fatal("ReceivedAt is zero, want publish timestamp")
	}

	if m.ReceivedAt.Before(before) || m.ReceivedAt.After(after) {
		t.Fatalf("ReceivedAt = %v, want within [%v, %v]", m.ReceivedAt, before, after)
	}
}

