package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/eventbus"
)

func TestRedactAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		addr string
		want string
	}{
		{"userinfo masked", "redis://user:hunter2@myhost:6379", "redis://user:xxxxx@myhost:6379"}, //nolint:gosec // fixture credential for redact unit test.
		{"plain hostport raw", "localhost:6379", "localhost:6379"},
		{"unparsable raw", "127.0.0.1:1", "127.0.0.1:1"},
		{"whitespace trimmed", "  localhost:6379  ", "localhost:6379"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := redactAddr(tc.addr); got != tc.want {
				t.Fatalf("redactAddr(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}

func TestPublish_canceledContext(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := b.Publish(ctx, freshTopic(), eventbus.NewPayload([]byte("x")), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish canceled ctx err = %v, want context.Canceled", err)
	}
}

func deadPortAdapter() *adapter {
	return &adapter{
		client:         goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"}), //nolint:gosec // dead-port fixture, no credential.
		prefix:         "eventbus",
		handlerTimeout: time.Second,
		closeTimeout:   time.Second,
		subs:           make(map[*subscription]struct{}),
	}
}

func TestPublish_brokerError(t *testing.T) {
	t.Parallel()

	a := deadPortAdapter()
	t.Cleanup(func() { _ = a.client.Close() })

	err := a.Publish(t.Context(), freshTopic(), eventbus.NewPayload([]byte("x")), nil)
	if err == nil || !strings.Contains(err.Error(), "redis: publish") {
		t.Fatalf("Publish dead broker err = %v, want redis: publish wrap", err)
	}
}

func TestSubscribe_receiveError(t *testing.T) {
	t.Parallel()

	a := deadPortAdapter()
	t.Cleanup(func() { _ = a.client.Close() })

	_, err := a.Subscribe(t.Context(), freshTopic(), func(context.Context, eventbus.Message) {})
	if err == nil || !strings.Contains(err.Error(), "redis: subscribe") {
		t.Fatalf("Subscribe dead broker err = %v, want redis: subscribe wrap", err)
	}
}

func TestSubscribe_closedAfterLock(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, nil)
	topic := freshTopic()

	a.mu.Lock()
	errCh := make(chan error, 1)

	go func() {
		_, err := a.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {})
		errCh <- err
	}()

	// The goroutine finishes the broker handshake then parks on a.mu,
	// which this test holds the whole time. Closing underneath exercises
	// the post-lock path deterministically without any timing wait.
	a.closed.Store(true)
	a.mu.Unlock()

	select {
	case err := <-errCh:
		if !errors.Is(err, eventbus.ErrClosed) {
			t.Fatalf("Subscribe post-lock err = %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for post-lock Subscribe")
	}
}

func TestDeliver_skipsBadFrames(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	a := newTestAdapter(t, nil)
	topic := freshTopic()

	got := make(chan eventbus.Message, 16)
	unsub, err := b.Subscribe(t.Context(), topic, func(_ context.Context, msg eventbus.Message) {
		got <- msg
	})
	if err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}
	defer unsub()

	raw := goredis.NewClient(&goredis.Options{Addr: testAddr})
	t.Cleanup(func() { _ = raw.Close() })

	ctx := t.Context()
	channel := a.channel(topic)

	oversize := make([]byte, eventbus.MaxMessageSize+1)
	if err := raw.Publish(ctx, channel, oversize).Err(); err != nil {
		t.Fatalf("raw oversize publish err = %v, want nil", err)
	}

	if err := raw.Publish(ctx, channel, "{not json").Err(); err != nil {
		t.Fatalf("raw invalid publish err = %v, want nil", err)
	}

	select {
	case m := <-got:
		t.Fatalf("received %q from bad frames, want nothing", m.Payload)
	case <-time.After(300 * time.Millisecond):
	}

	if err := b.Publish(ctx, topic, eventbus.NewPayload([]byte("still-alive")), nil); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	if m := waitMsg(t, got); string(m.Payload) != "still-alive" {
		t.Fatalf("Payload = %q, want %q", m.Payload, "still-alive")
	}
}

func TestDeliver_pubsubClosed(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, nil)
	topic := freshTopic()

	ps := a.client.Subscribe(t.Context(), a.channel(topic))
	sub := &subscription{ps: ps, topic: topic, channel: a.channel(topic), stop: make(chan struct{})}

	a.wg.Add(1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		a.deliver(t.Context(), sub, func(context.Context, eventbus.Message) {})
	}()

	// Whether deliver is already parked on select or not, ps.Close settles
	// the Go channel closed while stop stays open, so the !ok path is the
	// only ready case. No wait needed: deliver observes the close either way.
	_ = ps.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deliver did not return after PubSub close")
	}
}

func TestDecodeMessage(t *testing.T) {
	t.Parallel()

	topic := freshTopic()

	if _, err := decodeMessage(topic, make([]byte, eventbus.MaxMessageSize+1)); !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Errorf("oversize err = %v, want ErrPayloadTooLarge", err)
	}

	if _, err := decodeMessage(topic, []byte("{not json")); err == nil || !strings.Contains(err.Error(), "redis: decode") {
		t.Errorf("bad JSON err = %v, want redis: decode wrap", err)
	}

	badID, _ := json.Marshal(wireMessage{ID: "not-a-uuid"}) //nolint:errcheck // fixture marshal cannot fail.
	if _, err := decodeMessage(topic, badID); !errors.Is(err, eventbus.ErrInvalidMessageID) {
		t.Errorf("bad UUID err = %v, want ErrInvalidMessageID", err)
	}

	want := eventbus.NewMessage(topic, eventbus.NewPayload([]byte("hi")), eventbus.NewHeaders(map[string]string{"k": "v"}))
	raw, _ := json.Marshal(wireMessage{ID: want.ID.String(), Payload: want.Payload, Headers: want.Headers}) //nolint:errcheck // fixture marshal cannot fail.

	msg, err := decodeMessage(topic, raw)
	if err != nil {
		t.Fatalf("decode err = %v, want nil", err)
	}

	if msg.ID != want.ID || msg.Topic != topic || string(msg.Payload) != "hi" || msg.Headers["k"] != "v" {
		t.Fatalf("decoded = %+v, want ID/Topic/Payload/Headers roundtrip", msg)
	}
}

func TestClose_activeSub(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, nil)
	topic := freshTopic()

	if _, err := b.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {}); err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close with active sub err = %v, want nil", err)
	}

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrClosed) {
		t.Errorf("Publish after Close err = %v, want ErrClosed", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("second Close err = %v, want nil", err)
	}
}

func TestClose_timeoutArmed(t *testing.T) {
	t.Parallel()

	b := freshAdapter(t, func(o *eventbus.Options) { o.CloseTimeout = 20 * time.Millisecond })
	topic := freshTopic()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	if _, err := b.Subscribe(t.Context(), topic, func(context.Context, eventbus.Message) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}); err != nil {
		t.Fatalf("Subscribe err = %v, want nil", err)
	}

	if err := b.Publish(t.Context(), topic, eventbus.NewPayload([]byte("block")), nil); err != nil {
		t.Fatalf("Publish err = %v, want nil", err)
	}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never entered")
	}

	start := time.Now()
	if err := b.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Close took %v, want prompt return via closeTimeout", elapsed)
	}

	close(release)
}

func TestInvoke_panicNilOnPanic(t *testing.T) {
	t.Parallel()

	a := &adapter{handlerTimeout: time.Second}
	msg := eventbus.NewMessage("t", nil, nil)

	// Must recover silently instead of propagating when OnPanic is nil.
	a.invoke(t.Context(), "t", msg, func(context.Context, eventbus.Message) { panic("boom") })

	called := false
	a.invoke(t.Context(), "t", msg, func(context.Context, eventbus.Message) { called = true })

	if !called {
		t.Fatal("handler not called")
	}
}

func TestSubscription_shutdownIdempotent(t *testing.T) {
	t.Parallel()

	s := &subscription{stop: make(chan struct{})}
	s.shutdown()
	s.shutdown()

	select {
	case <-s.stop:
	default:
		t.Fatal("stop not closed after shutdown")
	}
}
