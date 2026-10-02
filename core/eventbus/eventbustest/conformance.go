// Package eventbustest provides the conformance kit third-party eventbus adapters run to prove backend parity.
package eventbustest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/eventbus"
)

const (
	// DefaultDeliveryTimeout bounds how long delivery polls wait before failing.
	DefaultDeliveryTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between delivery-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

// Conformance verifies factory-built buses implement the
// eventbus.EventBus contract: open/register round-trip, push
// publish/subscribe delivery with payload+header fidelity and
// unsubscribe, pull SubscribeChan/Unsubscribe, invalid topic and
// payload sentinels, topic isolation, and Close. Each subtest takes a
// fresh instance from factory so cases stay isolated. Delivery waits
// poll with a context deadline; they never synchronize with
// time.Sleep and never touch the network.
//
// No-op adapters: none; every adapter must deliver messages.
// A stub that drops every publish would trivially satisfy the shape
// but violate delivery, so no stub exemption exists.
func Conformance(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("PushDelivery", func(t *testing.T) { conformancePushDelivery(t, factory) })
	t.Run("Unsubscribe", func(t *testing.T) { conformanceUnsubscribe(t, factory) })
	t.Run("PullChannel", func(t *testing.T) { conformancePullChannel(t, factory) })
	t.Run("TopicIsolation", func(t *testing.T) { conformanceTopicIsolation(t, factory) })
	t.Run("InvalidInput", func(t *testing.T) { conformanceInvalidInput(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := eventbus.Open(eventbus.Adapter("conformance-missing-adapter"), eventbus.Options{}); !errors.Is(err, eventbus.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := eventbus.Adapter("conformance-probe-eventbus")

	if err := eventbus.Register(probe, nil); !errors.Is(err, eventbus.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(eventbus.Options) (eventbus.EventBus, error) {
		return nil, errors.New("eventbustest: probe factory must not run")
	}

	_ = eventbus.Register(probe, stub)

	if err := eventbus.Register(probe, stub); !errors.Is(err, eventbus.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformancePushDelivery(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if b.Name() == "" {
		t.Error("Name() is empty")
	}

	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, "kit-push", func(_ context.Context, msg eventbus.Message) {
		select {
		case got <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsub()

	payload := eventbus.Payload([]byte("hello-conformance"))
	if err := b.Publish(ctx, "kit-push", payload, eventbus.Headers{"k": "v"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	payload[0] = 'X'

	msg := awaitMessage(t, got)

	if string(msg.Payload) != "hello-conformance" {
		t.Errorf("Payload = %q, want stored copy", msg.Payload)
	}

	if msg.Headers["k"] != "v" {
		t.Errorf("Headers[k] = %q, want v", msg.Headers["k"])
	}

	if msg.Topic != "kit-push" {
		t.Errorf("Topic = %q, want kit-push", msg.Topic)
	}
}

func conformanceUnsubscribe(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, "kit-unsub", func(_ context.Context, msg eventbus.Message) {
		select {
		case got <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	unsub()
	unsub()

	if err := b.Publish(ctx, "kit-unsub", eventbus.Payload([]byte("after")), nil); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case msg := <-got:
		t.Fatalf("received %+v after unsubscribe, want silence", msg)
	case <-pollDone(ctx, 3*DefaultPollInterval):
	}

	if _, err := b.Subscribe(ctx, "kit-unsub", nil); !errors.Is(err, eventbus.ErrNilHandler) {
		t.Errorf("Subscribe(nil) err = %v, want ErrNilHandler", err)
	}
}

func conformancePullChannel(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	ch, err := b.SubscribeChan(ctx, "kit-pull", 4)
	if err != nil {
		t.Fatalf("SubscribeChan() error = %v", err)
	}

	if err := b.Publish(ctx, "kit-pull", eventbus.Payload([]byte("pull-me")), eventbus.Headers{"h": "1"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case msg := <-ch:
		if string(msg.Payload) != "pull-me" {
			t.Errorf("Payload = %q, want pull-me", msg.Payload)
		}
	case <-pollDone(ctx, DefaultDeliveryTimeout):
		t.Fatal("pull message not delivered within timeout")
	}

	if err := b.Unsubscribe("kit-pull", ch); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}

	if err := b.Unsubscribe("kit-pull", ch); !errors.Is(err, eventbus.ErrNotSubscribed) {
		t.Errorf("Unsubscribe(again) err = %v, want ErrNotSubscribed", err)
	}
}

func conformanceTopicIsolation(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, "kit-a", func(_ context.Context, msg eventbus.Message) {
		select {
		case got <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsub()

	if err := b.Publish(ctx, "kit-b", eventbus.Payload([]byte("wrong-topic")), nil); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case msg := <-got:
		t.Fatalf("received %+v on kit-a for a kit-b publish, want isolation", msg)
	case <-pollDone(ctx, 3*DefaultPollInterval):
	}
}

func conformanceInvalidInput(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if err := b.Publish(ctx, "", eventbus.Payload([]byte("x")), nil); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("Publish(empty topic) err = %v, want ErrInvalidOptions", err)
	}

	if err := b.Publish(ctx, strings.Repeat("t", eventbus.MaxTopicLen+1), eventbus.Payload([]byte("x")), nil); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("Publish(long topic) err = %v, want ErrInvalidOptions", err)
	}

	huge := make(eventbus.Payload, eventbus.MaxMessageSize+1)
	if err := b.Publish(ctx, "kit-big", huge, nil); !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Errorf("Publish(huge) err = %v, want ErrPayloadTooLarge", err)
	}

	if _, err := b.Subscribe(ctx, "", func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Errorf("Subscribe(empty) err = %v, want ErrInvalidOptions", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) eventbus.EventBus) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if err := b.Publish(ctx, "kit-push", eventbus.Payload([]byte("x")), nil); !errors.Is(err, eventbus.ErrClosed) {
		t.Errorf("Publish() err = %v, want ErrClosed", err)
	}

	if _, err := b.Subscribe(ctx, "kit-push", func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrClosed) {
		t.Errorf("Subscribe() err = %v, want ErrClosed", err)
	}
}

// awaitMessage polls got until a message arrives or the deadline lapses.
func awaitMessage(t *testing.T, got <-chan eventbus.Message) eventbus.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultDeliveryTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-got:
			return msg
		case <-ctx.Done():
			t.Fatal("message not delivered within timeout")
			return eventbus.Message{}
		case <-ticker.C:
		}
	}
}

// pollDone fires once after d without time.Sleep synchronization.
func pollDone(ctx context.Context, d time.Duration) <-chan struct{} {
	ch := make(chan struct{}, 1)

	go func() {
		t := time.NewTimer(d)
		defer t.Stop()

		select {
		case <-ctx.Done():
		case <-t.C:
			ch <- struct{}{}
		}
	}()

	return ch
}
