package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/eventbus"
)

// topicSeq keeps generated topics unique even within a single test's shared
// miniredis instance.
var topicSeq atomic.Int64

func freshTopic() string {
	return fmt.Sprintf("t%d", topicSeq.Add(1))
}

// testServer starts a per-test miniredis instance, auto-closed via
// t.Cleanup.
func testServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	return miniredis.RunT(t)
}

// optionsFor builds eventbus.Options pointed at an already-running server,
// for tests that need multiple adapters sharing one broker.
func optionsFor(s *miniredis.Miniredis) eventbus.Options {
	return eventbus.Options{Redis: eventbus.RedisOptions{Addr: s.Addr()}}
}

func testOptions(t *testing.T) eventbus.Options {
	t.Helper()

	return optionsFor(testServer(t))
}

// freshAdapter builds an adapter on its own isolated server, unless mutate
// or a shared server is needed — see freshAdapterOn.
func freshAdapter(t *testing.T, mutate func(*eventbus.Options)) eventbus.EventBus {
	t.Helper()

	return freshAdapterOn(t, testServer(t), mutate)
}

// freshAdapterOn builds an adapter on the given (possibly shared) server.
func freshAdapterOn(t *testing.T, s *miniredis.Miniredis, mutate func(*eventbus.Options)) eventbus.EventBus {
	t.Helper()

	opts := optionsFor(s)
	if mutate != nil {
		mutate(&opts)
	}

	b, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})

	return b
}

func newTestAdapter(t *testing.T, mutate func(*eventbus.Options)) *adapter {
	t.Helper()

	return newTestAdapterOn(t, testServer(t), mutate)
}

func newTestAdapterOn(t *testing.T, s *miniredis.Miniredis, mutate func(*eventbus.Options)) *adapter {
	t.Helper()

	opts := optionsFor(s)
	if mutate != nil {
		mutate(&opts)
	}

	a, err := newAdapter(opts)
	if err != nil {
		t.Fatalf("newAdapter err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})

	return a
}

func waitMsg(t *testing.T, ch <-chan eventbus.Message) eventbus.Message {
	t.Helper()

	select {
	case m := <-ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
		return eventbus.Message{}
	}
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*eventbus.Options)
	}{
		{"negative buffer", func(o *eventbus.Options) { o.BufferSize = -1 }},
		{"negative max handlers", func(o *eventbus.Options) { o.MaxHandlers = -1 }},
		{"negative handler timeout", func(o *eventbus.Options) { o.HandlerTimeout = -time.Second }},
		{"negative close timeout", func(o *eventbus.Options) { o.CloseTimeout = -time.Second }},
		{"scheme addr", func(o *eventbus.Options) { o.Redis.Addr = "redis://localhost:6379" }},
		{"bad port addr", func(o *eventbus.Options) { o.Redis.Addr = "localhost:notaport" }},
		{"oversize prefix", func(o *eventbus.Options) { o.Redis.Prefix = strings.Repeat("a", 65) }},
		{"bad prefix chars", func(o *eventbus.Options) { o.Redis.Prefix = "has space" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			tc.mutate(&opts)

			if _, err := New(opts); !errors.Is(err, eventbus.ErrInvalidOptions) {
				t.Fatalf("New err = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

func TestNew_nameIsRedis(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	if got := b.Name(); got != "redis" {
		t.Fatalf("Name() = %q, want %q", got, "redis")
	}
}

func TestPublishSubscribe_roundtrip(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	got := make(chan eventbus.Message, 16)
	unsub, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	defer unsub()

	headers := eventbus.NewHeaders(map[string]string{"k": "v"})
	payload := eventbus.NewPayload([]byte("hello"))

	if err := b.Publish(t.Context(), topic, payload, headers); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	m := waitMsg(t, got)

	if m.Topic != topic {
		t.Errorf("Topic = %q, want %q", m.Topic, topic)
	}

	if string(m.Payload) != "hello" {
		t.Errorf("Payload = %q, want %q", m.Payload, "hello")
	}

	if m.Headers["k"] != "v" {
		t.Errorf("Headers = %v, want k=v", m.Headers)
	}

	if _, err := eventbus.ParseMessageID(m.ID.String()); err != nil {
		t.Errorf("ID %q does not parse: %v", m.ID.String(), err)
	}
}

func TestPublishSubscribe_ordered(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	got := make(chan eventbus.Message, 64)
	unsub, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	defer unsub()

	const n = 20

	for i := range n {
		p := eventbus.NewPayload([]byte(fmt.Sprintf("msg-%02d", i)))
		if err := b.Publish(t.Context(), topic, p, nil); err != nil {
			t.Fatalf("Publish %d err = %v, want nil", i, err)
		}
	}

	for i := range n {
		m := waitMsg(t, got)
		if want := fmt.Sprintf("msg-%02d", i); string(m.Payload) != want {
			t.Fatalf("message %d Payload = %q, want %q (ordering broken)", i, m.Payload, want)
		}
	}
}

func TestPublish_fanout(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	ch1 := make(chan eventbus.Message, 4)
	ch2 := make(chan eventbus.Message, 4)

	unsub1, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		ch1 <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe 1 err = %v, want nil", err)
	}
	defer unsub1()

	unsub2, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		ch2 <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe 2 err = %v, want nil", err)
	}
	defer unsub2()

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("fan")), nil); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	m1 := waitMsg(t, ch1)
	m2 := waitMsg(t, ch2)

	if m1.ID != m2.ID {
		t.Errorf("fanout IDs differ: %v vs %v, want same message", m1.ID, m2.ID)
	}
}

func TestPublish_noSubscribers_dropsSilently(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)

	if err := b.Publish(t.Context(), freshTopic(), eventbus.NewPayload([]byte("drop")), nil); err != nil {
		t.Fatalf("Publish with no subscribers err = %v, want nil", err)
	}
}

func TestUnsubscribe_stopsDelivery(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	got := make(chan eventbus.Message, 16)
	unsub, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("one")), nil); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	waitMsg(t, got)
	unsub()

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("two")), nil); err != nil {
		t.Fatalf("Publish after unsubscribe err = %v, want nil", err)
	}

	select {
	case m := <-got:
		t.Fatalf("received %q after unsubscribe, want nothing", m.Payload)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestPublish_oversizePayload(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	huge := eventbus.NewPayload(make([]byte, eventbus.MaxMessageSize+1))
	if err := b.Publish(t.Context(), topic, huge, nil); !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Errorf("raw oversize err = %v, want ErrPayloadTooLarge", err)
	}

	// Raw payload at exactly the limit still exceeds it once wrapped in
	// the JSON envelope (id + headers + base64 overhead).
	edge := eventbus.NewPayload(make([]byte, eventbus.MaxMessageSize))
	if err := b.Publish(t.Context(), topic, edge, nil); !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Errorf("envelope oversize err = %v, want ErrPayloadTooLarge", err)
	}
}

func TestSubscribe_nilHandler(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)

	if _, err := b.Subscribe(t.Context(), freshTopic(), nil); !errors.Is(err, eventbus.ErrNilHandler) {
		t.Fatalf("Subscribe nil handler err = %v, want ErrNilHandler", err)
	}
}

func TestSubscribe_invalidTopic(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)

	if _, err := b.Subscribe(t.Context(), "", func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("empty topic err = %v, want ErrInvalidOptions", err)
	}

	long := strings.Repeat("a", eventbus.MaxTopicLen+1)
	if _, err := b.Subscribe(t.Context(), long, func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("oversize topic err = %v, want ErrInvalidOptions", err)
	}

	if err := b.Publish(t.Context(), "", eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("publish empty topic err = %v, want ErrInvalidOptions", err)
	}
}

func TestClose_behavior(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	got := make(chan eventbus.Message, 4)
	unsub, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	unsub()

	if err := b.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrClosed) {
		t.Errorf("Publish after Close err = %v, want ErrClosed", err)
	}

	if _, err := b.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrClosed) {
		t.Errorf("Subscribe after Close err = %v, want ErrClosed", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("second Close err = %v, want nil", err)
	}
}

func TestSubscribe_onPanic_survives(t *testing.T) {
	t.Parallel()

	var panicked atomic.Int64

	b := freshAdapter(t, func(o *eventbus.Options) {
		o.OnPanic = func(_ string, _ eventbus.Message, _ any) { panicked.Add(1) }
	})
	topic := freshTopic()

	got := make(chan eventbus.Message, 16)

	unsub1, err := b.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {
		panic("boom")
	})
	if err != nil {
		t.Fatalf("Subscribe panicking err = %v, want nil", err)
	}
	defer unsub1()

	unsub2, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	defer unsub2()

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	waitMsg(t, got)

	eventually(t, 5*time.Second, func() bool { return panicked.Load() != 0 }, "OnPanic after handler panic")
}

func TestPublishSubscribe_concurrent(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	const publishers = 4
	const perPub = 25

	var received atomic.Int64

	unsub, err := b.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {
		received.Add(1)
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	defer unsub()

	var wg sync.WaitGroup

	for p := range publishers {
		wg.Add(1)

		go func(p int) {
			defer wg.Done()

			for i := range perPub {
				payload := eventbus.NewPayload([]byte(fmt.Sprintf("p%d-%d", p, i)))
				if err := b.Publish(t.Context(), topic, payload, nil); err != nil {
					t.Errorf("Publish err = %v, want nil", err)
					return
				}
			}
		}(p)
	}

	wg.Wait()

	eventually(t, 10*time.Second, func() bool { return received.Load() == publishers*perPub }, "concurrent deliveries")
}

func TestPrefix_isolation(t *testing.T) {
	t.Parallel()

	topic := freshTopic()
	server := testServer(t)

	busA := freshAdapterOn(t, server, func(o *eventbus.Options) { o.Redis.Prefix = "pa" })
	busB := freshAdapterOn(t, server, func(o *eventbus.Options) { o.Redis.Prefix = "pb" })

	gotB := make(chan eventbus.Message, 4)
	unsubB, err := busB.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		gotB <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe B err = %v, want nil", err)
	}
	defer unsubB()

	gotA := make(chan eventbus.Message, 4)
	unsubA, err := busA.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		gotA <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe A err = %v, want nil", err)
	}
	defer unsubA()

	if err := busA.Publish(t.Context(), topic, eventbus.NewPayload([]byte("a")), nil); err != nil {
		t.Fatalf("Publish A err = %v, want nil", err)
	}

	waitMsg(t, gotA)

	select {
	case m := <-gotB:
		t.Fatalf("prefix leak: B received %q published on A", m.Payload)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestNew_unreachablePing runs last and sequentially: it points the shared
// Pool singleton at a dead address, then restores the test server client.
func TestNew_unreachablePing(t *testing.T) {
	opts := eventbus.Options{Redis: eventbus.RedisOptions{Addr: "127.0.0.1:1"}}

	_, err := New(opts)
	if err == nil {
		t.Fatal("New unreachable err = nil, want ping failure")
	}

	if !strings.Contains(err.Error(), "redis: ping") {
		t.Fatalf("New unreachable err = %v, want it to mention redis: ping", err)
	}

	if _, err := New(testOptions(t)); err != nil {
		t.Fatalf("New restore err = %v, want nil", err)
	}
}
