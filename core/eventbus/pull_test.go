package eventbus

import (
	"context"
	"errors"
	"fmt"
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

func openPullBus(t *testing.T, inner Pusher) EventBus {
	t.Helper()

	a := Adapter(fmt.Sprintf("test-%d", 2000+int(freshSeq.Add(1))))
	if err := Register(a, func(Options) (EventBus, error) { return Wrap(inner), nil }); err != nil {
		t.Fatalf("Register err = %v, want nil", err)
	}

	b, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = b.Close() })

	return b
}

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

func TestSubscribeChan_fanoutBothReceive(t *testing.T) {
	t.Parallel()

	inner := newFakeBus()
	b := openPullBus(t, inner)
	ctx := t.Context()

	c1, err := b.SubscribeChan(ctx, "orders", 16)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	c2, err := b.SubscribeChan(ctx, "orders", 16)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	if err := b.Publish(ctx, "orders", NewPayload([]byte("hi")), NewHeaders(map[string]string{"k": "v"})); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	m1 := recvChan(t, c1)
	m2 := recvChan(t, c2)

	if string(m1.Payload) != "hi" || m1.Topic != "orders" || m1.Headers["k"] != "v" {
		t.Fatalf("c1 got %+v, want orders/hi", m1)
	}

	if m1.ID != m2.ID {
		t.Fatalf("fanout IDs differ: %v vs %v", m1.ID, m2.ID)
	}

	if m2.ReceivedAt.IsZero() {
		t.Fatal("forwarded ReceivedAt is zero, want publish timestamp preserved")
	}
}

func TestSubscribeChan_slowChanDropIsolation(t *testing.T) {
	t.Parallel()

	inner := newFakeBus()
	b := openPullBus(t, inner)
	ctx := t.Context()

	slow, err := b.SubscribeChan(ctx, "orders", 1)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	fast, err := b.SubscribeChan(ctx, "orders", 16)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	for _, body := range []string{"m1", "m2", "m3"} {
		if err := b.Publish(ctx, "orders", NewPayload([]byte(body)), nil); err != nil {
			t.Fatalf("Publish err = %v, want nil", err)
		}
	}

	for _, want := range []string{"m1", "m2", "m3"} {
		if got := recvChan(t, fast); string(got.Payload) != want {
			t.Fatalf("fast got %q, want %q", got.Payload, want)
		}
	}

	select {
	case m := <-slow:
		if string(m.Payload) != "m1" {
			t.Fatalf("slow first = %q, want %q (oldest retained)", m.Payload, "m1")
		}
	default:
		t.Fatal("slow chan empty, want first message buffered")
	}
}

func TestSubscribeChan_invalidTopic(t *testing.T) {
	t.Parallel()

	b := openPullBus(t, newFakeBus())
	ctx := t.Context()

	if _, err := b.SubscribeChan(ctx, "", 8); err == nil {
		t.Fatal("SubscribeChan empty topic err = nil, want validation error")
	}
}

func TestSubscribeChan_defaultBuffer(t *testing.T) {
	t.Parallel()

	b := openPullBus(t, newFakeBus())
	ctx := t.Context()

	for _, buf := range []int{0, -1} {
		ch, err := b.SubscribeChan(ctx, "orders", buf)
		if err != nil {
			t.Fatalf("SubscribeChan buffer %d err = %v, want nil", buf, err)
		}

		if err := b.Publish(ctx, "orders", NewPayload([]byte("hi")), nil); err != nil {
			t.Fatalf("Publish err = %v, want nil", err)
		}

		if got := recvChan(t, ch); string(got.Payload) != "hi" {
			t.Fatalf("got %q, want %q", got.Payload, "hi")
		}

		if err := b.Unsubscribe("orders", ch); err != nil {
			t.Fatalf("Unsubscribe err = %v, want nil", err)
		}
	}
}

func TestUnsubscribe_unknown(t *testing.T) {
	t.Parallel()

	b := openPullBus(t, newFakeBus())

	if err := b.Unsubscribe("orders", make(chan Message, 1)); !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("Unsubscribe unknown err = %v, want ErrNotSubscribed", err)
	}
}

func TestUnsubscribe_wrongTopic(t *testing.T) {
	t.Parallel()

	b := openPullBus(t, newFakeBus())
	ctx := t.Context()

	ch, err := b.SubscribeChan(ctx, "orders", 4)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	if err := b.Unsubscribe("payments", ch); !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("Unsubscribe wrong topic err = %v, want ErrNotSubscribed", err)
	}

	if err := b.Unsubscribe("orders", ch); err != nil {
		t.Fatalf("Unsubscribe correct topic err = %v, want nil", err)
	}
}

func TestUnsubscribe_twice(t *testing.T) {
	t.Parallel()

	inner := newFakeBus()
	b := openPullBus(t, inner)
	ctx := t.Context()

	ch, err := b.SubscribeChan(ctx, "orders", 4)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	if err := b.Unsubscribe("orders", ch); err != nil {
		t.Fatalf("first Unsubscribe err = %v, want nil", err)
	}

	if inner.unsubbed != 1 {
		t.Fatalf("inner unsubs = %d, want 1", inner.unsubbed)
	}

	if err := b.Unsubscribe("orders", ch); !errors.Is(err, ErrNotSubscribed) {
		t.Fatalf("second Unsubscribe err = %v, want ErrNotSubscribed", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("recv from unsubscribed chan, want closed")
		}
	default:
		t.Fatal("unsubscribed chan not closed")
	}
}

func TestPull_afterClose(t *testing.T) {
	t.Parallel()

	inner := newFakeBus()
	b := openPullBus(t, inner)
	ctx := t.Context()

	ch, err := b.SubscribeChan(ctx, "orders", 4)
	if err != nil {
		t.Fatalf("SubscribeChan err = %v, want nil", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("recv from post-Close chan, want closed")
		}
	default:
		t.Fatal("post-Close chan not closed")
	}

	if _, err := b.SubscribeChan(ctx, "orders", 4); !errors.Is(err, ErrClosed) {
		t.Fatalf("SubscribeChan after Close err = %v, want ErrClosed", err)
	}

	if err := b.Unsubscribe("orders", ch); !errors.Is(err, ErrClosed) {
		t.Fatalf("Unsubscribe after Close err = %v, want ErrClosed", err)
	}
}
