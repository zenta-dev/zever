package eventbus

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// stickyBus is a fakeBus whose unsubscribe is a no-op: the push handler
// stays registered so late publishes still reach the wrapper. It pins the
// wrapper's closed-subscriber early return deterministically.
type stickyBus struct {
	*fakeBus
}

// Subscribe registers handler and returns a no-op detach function.
func (b *stickyBus) Subscribe(ctx context.Context, topic string, handler Handler) (func(), error) {
	if _, err := b.fakeBus.Subscribe(ctx, topic, handler); err != nil {
		return nil, err
	}

	return func() {}, nil
}

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise the adapter
// errors via duplicate/unknown/invalid paths, so direct unit tests now pin
// the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSubs []string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "mem"}, ErrDuplicateAdapter, []string{"mem"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "mem"}, ErrUnknownAdapter, []string{"mem"}, &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad topic"}, ErrInvalidOptions, []string{"bad topic"}, &InvalidOptionsError{}},
		{"msgid", InvalidMessageIDError{ID: "bogus"}, ErrInvalidMessageID, []string{"bogus"}, &InvalidMessageIDError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "eventbus: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "eventbus: ")
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("Error() = %q, want substring %q", msg, sub)
				}
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}

// TestRegRemovalInvalidMessageIDCause covers the InvalidMessageIDError
// message and joined Unwrap when a parse cause is present.
func TestRegRemovalInvalidMessageIDCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("short id")
	err := InvalidMessageIDError{ID: "bogus", Err: cause}

	if got := err.Error(); !strings.Contains(got, "short id") {
		t.Errorf("Error() = %q, want cause", got)
	}

	if !errors.Is(err, ErrInvalidMessageID) {
		t.Errorf("errors.Is(err, ErrInvalidMessageID) = false")
	}

	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false")
	}
}

// TestRegRemovalPullWrapper covers the pullBus upgrade wrapper: push
// passthrough, name, pull-channel delivery, buffer default, full-channel
// drop, unsubscribe paths, and idempotent close. The conformance kit only
// drives adapters through the EventBus interface, so these pin the wrapper
// itself.
func TestRegRemovalPullWrapper(t *testing.T) {
	t.Parallel()

	t.Run("passthrough", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		bus := Wrap(newFakeBus())
		defer bus.Close()

		if got := bus.Name(); got != "fake" {
			t.Errorf("Name() = %q, want fake", got)
		}

		var got atomic.Int64

		unsub, err := bus.Subscribe(ctx, "orders", func(_ context.Context, msg Message) {
			if string(msg.Payload) == "hi" {
				got.Add(1)
			}
		})
		if err != nil {
			t.Fatalf("Subscribe() err = %v", err)
		}

		if err := bus.Publish(ctx, "orders", NewPayload([]byte("hi")), nil); err != nil {
			t.Fatalf("Publish() err = %v", err)
		}

		if got.Load() != 1 {
			t.Errorf("handler calls = %d, want 1", got.Load())
		}

		unsub()
	})

	t.Run("pull/deliver", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		bus := Wrap(newFakeBus())

		ch, err := bus.SubscribeChan(ctx, "orders", 0)
		if err != nil {
			t.Fatalf("SubscribeChan() err = %v", err)
		}

		if got := cap(ch); got != DefaultChanBuffer {
			t.Errorf("cap(ch) = %d, want DefaultChanBuffer %d", got, DefaultChanBuffer)
		}

		if err := bus.Publish(ctx, "orders", NewPayload([]byte("hi")), nil); err != nil {
			t.Fatalf("Publish() err = %v", err)
		}

		msg := recvChan(t, ch)
		if string(msg.Payload) != "hi" {
			t.Errorf("payload = %q, want hi", msg.Payload)
		}

		if err := bus.Unsubscribe("orders", ch); err != nil {
			t.Fatalf("Unsubscribe() err = %v", err)
		}

		if err := bus.Unsubscribe("orders", ch); !errors.Is(err, ErrNotSubscribed) {
			t.Errorf("Unsubscribe() err = %v, want ErrNotSubscribed", err)
		}

		if err := bus.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}

		if err := bus.Close(); err != nil {
			t.Fatalf("second Close() err = %v", err)
		}
	})

	t.Run("pull/invalid-topic", func(t *testing.T) {
		t.Parallel()

		bus := Wrap(newFakeBus())
		defer bus.Close()

		if _, err := bus.SubscribeChan(t.Context(), "", 1); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("SubscribeChan(\"\") err = %v, want ErrInvalidOptions", err)
		}
	})

	t.Run("pull/drop-full", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		bus := Wrap(newFakeBus())
		defer bus.Close()

		ch, err := bus.SubscribeChan(ctx, "orders", 1)
		if err != nil {
			t.Fatalf("SubscribeChan() err = %v", err)
		}

		for _, body := range []string{"one", "two"} {
			if err := bus.Publish(ctx, "orders", NewPayload([]byte(body)), nil); err != nil {
				t.Fatalf("Publish(%q) err = %v", body, err)
			}
		}

		if got := recvChan(t, ch); string(got.Payload) != "one" {
			t.Errorf("payload = %q, want one (newest must drop)", got.Payload)
		}

		if err := bus.Unsubscribe("wrong", ch); !errors.Is(err, ErrNotSubscribed) {
			t.Errorf("Unsubscribe(wrong topic) err = %v, want ErrNotSubscribed", err)
		}
	})

	t.Run("closed", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		bus := Wrap(newFakeBus())

		if err := bus.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}

		if _, err := bus.SubscribeChan(ctx, "orders", 1); !errors.Is(err, ErrClosed) {
			t.Errorf("SubscribeChan() err = %v, want ErrClosed", err)
		}

		if err := bus.Unsubscribe("orders", make(chan Message)); !errors.Is(err, ErrClosed) {
			t.Errorf("Unsubscribe() err = %v, want ErrClosed", err)
		}

		if err := bus.Publish(ctx, "orders", NewPayload([]byte("x")), nil); !errors.Is(err, ErrClosed) {
			t.Errorf("Publish() err = %v, want ErrClosed", err)
		}
	})

	t.Run("pull/publish-after-detach", func(t *testing.T) {
		t.Parallel()

		// stickyBus keeps the push handler registered after unsubscribe
		// so a late Publish exercises the wrapper's closed-subscriber
		// early return instead of sending on a closed channel.
		ctx := t.Context()
		bus := Wrap(&stickyBus{fakeBus: newFakeBus()})

		ch, err := bus.SubscribeChan(ctx, "orders", 1)
		if err != nil {
			t.Fatalf("SubscribeChan() err = %v", err)
		}

		if err := bus.Unsubscribe("orders", ch); err != nil {
			t.Fatalf("Unsubscribe() err = %v", err)
		}

		if err := bus.Publish(ctx, "orders", NewPayload([]byte("late")), nil); err != nil {
			t.Fatalf("Publish() err = %v", err)
		}

		if err := bus.Close(); err != nil {
			t.Fatalf("Close() err = %v", err)
		}
	})
}
