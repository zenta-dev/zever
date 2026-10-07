package outboxbridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/queue"
)

// push is one recorded transport call.
type push struct {
	Topic   string
	Payload []byte
	Headers map[string]string
}

// stubQueue records every Push and can fail on demand.
type stubQueue struct {
	pushes []push
	err    error
}

func (q *stubQueue) Push(_ context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	if q.err != nil {
		return q.err
	}

	q.pushes = append(q.pushes, push{Topic: topic, Payload: payload, Headers: headers})

	return nil
}

func (q *stubQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (q *stubQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (q *stubQueue) Ack(_ context.Context, _ queue.Message) error          { return nil }
func (q *stubQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (q *stubQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (q *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (q *stubQueue) Close() error { return nil }

func (q *stubQueue) Name() string { return "stub" }

// stubEventBus records every Publish and can fail on demand.
type stubEventBus struct {
	pushes []push
	err    error
}

func (b *stubEventBus) Publish(_ context.Context, topic string, payload eventbus.Payload, headers eventbus.Headers) error {
	if b.err != nil {
		return b.err
	}

	b.pushes = append(b.pushes, push{Topic: topic, Payload: payload, Headers: headers})

	return nil
}

func (b *stubEventBus) Subscribe(_ context.Context, _ string, _ eventbus.Handler) (func(), error) {
	return func() {}, nil
}

func (b *stubEventBus) SubscribeChan(_ context.Context, _ string, _ int) (<-chan eventbus.Message, error) {
	ch := make(chan eventbus.Message)

	return ch, nil
}

func (b *stubEventBus) Unsubscribe(_ string, _ <-chan eventbus.Message) error { return nil }

func (b *stubEventBus) Close() error { return nil }

func (b *stubEventBus) Name() string { return "stub" }

var (
	_ queue.Queue       = (*stubQueue)(nil)
	_ eventbus.EventBus = (*stubEventBus)(nil)
	_ outbox.Publisher  = queuePublisher{}
	_ outbox.Publisher  = eventBusPublisher{}
)

// sampleMessage is the message every happy-path test publishes.
func sampleMessage() outbox.Message {
	return outbox.Message{
		ID:      "msg-1",
		Topic:   "orders.created",
		Key:     "tenant-42",
		Payload: []byte(`{"id":"o-1"}`),
		Headers: map[string]string{"traceparent": "00-abc-def-01"},
	}
}

// The queue bridge pushes the message onto its topic as a queue message; the
// table pins that mapping for both transports against one set of assertions.
func TestPublisher_mapsTopicPayloadHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   func(t *testing.T) (outbox.Publisher, func() []push)
		wantLen int
	}{
		{
			name: "queue",
			build: func(t *testing.T) (outbox.Publisher, func() []push) {
				t.Helper()

				q := &stubQueue{}

				p, err := QueuePublisher(q)
				if err != nil {
					t.Fatalf("QueuePublisher failed: %v", err)
				}

				return p, func() []push { return q.pushes }
			},
			wantLen: 1,
		},
		{
			name: "eventbus",
			build: func(t *testing.T) (outbox.Publisher, func() []push) {
				t.Helper()

				bus := &stubEventBus{}

				p, err := EventBusPublisher(bus)
				if err != nil {
					t.Fatalf("EventBusPublisher failed: %v", err)
				}

				return p, func() []push { return bus.pushes }
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, pushes := tt.build(t)

			msg := sampleMessage()
			if err := p.Publish(t.Context(), msg); err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			recorded := pushes()
			if len(recorded) != tt.wantLen {
				t.Fatalf("recorded %d messages, want %d", len(recorded), tt.wantLen)
			}

			got := recorded[0]

			if got.Topic != msg.Topic {
				t.Errorf("topic = %q, want %q", got.Topic, msg.Topic)
			}

			if string(got.Payload) != string(msg.Payload) {
				t.Errorf("payload = %q, want %q", got.Payload, msg.Payload)
			}

			if got.Headers["traceparent"] != msg.Headers["traceparent"] {
				t.Errorf("headers = %v, want traceparent %q", got.Headers, msg.Headers["traceparent"])
			}
		})
	}
}

// The queue has no routing-key argument, so Key rides in the headers. Both
// transports behave the same way; the test table pins one contract for both.
func TestPublisher_keyLandsInHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		build  func(t *testing.T) (outbox.Publisher, func() map[string]string)
		wantNo string
	}{
		{
			name: "queue",
			build: func(t *testing.T) (outbox.Publisher, func() map[string]string) {
				t.Helper()

				q := &stubQueue{}

				p, err := QueuePublisher(q)
				if err != nil {
					t.Fatalf("QueuePublisher failed: %v", err)
				}

				return p, func() map[string]string {
					if len(q.pushes) != 1 {
						t.Fatalf("queue recorded %d pushes, want 1", len(q.pushes))
					}

					return q.pushes[0].Headers
				}
			},
		},
		{
			name: "eventbus",
			build: func(t *testing.T) (outbox.Publisher, func() map[string]string) {
				t.Helper()

				bus := &stubEventBus{}

				p, err := EventBusPublisher(bus)
				if err != nil {
					t.Fatalf("EventBusPublisher failed: %v", err)
				}

				return p, func() map[string]string {
					if len(bus.pushes) != 1 {
						t.Fatalf("eventbus recorded %d publishes, want 1", len(bus.pushes))
					}

					return bus.pushes[0].Headers
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, headers := tt.build(t)

			if err := p.Publish(t.Context(), sampleMessage()); err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			got := headers()

			if got[KeyHeader] != "tenant-42" {
				t.Errorf("%s = %q, want %q", KeyHeader, got[KeyHeader], "tenant-42")
			}

			if key := Key(got); key != "tenant-42" {
				t.Errorf("Key(headers) = %q, want %q", key, "tenant-42")
			}
		})
	}
}

// A message with neither headers nor a key carries none, so transports that
// special-case an empty map see nil rather than an allocated map.
func TestPublisher_noHeadersNoKeySendsNil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T) (outbox.Publisher, func() map[string]string)
	}{
		{
			name: "queue",
			build: func(t *testing.T) (outbox.Publisher, func() map[string]string) {
				t.Helper()

				q := &stubQueue{}

				p, err := QueuePublisher(q)
				if err != nil {
					t.Fatalf("QueuePublisher failed: %v", err)
				}

				return p, func() map[string]string { return q.pushes[0].Headers }
			},
		},
		{
			name: "eventbus",
			build: func(t *testing.T) (outbox.Publisher, func() map[string]string) {
				t.Helper()

				bus := &stubEventBus{}

				p, err := EventBusPublisher(bus)
				if err != nil {
					t.Fatalf("EventBusPublisher failed: %v", err)
				}

				return p, func() map[string]string { return bus.pushes[0].Headers }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, headers := tt.build(t)

			msg := outbox.Message{ID: "msg-2", Topic: "orders.created", Payload: []byte("body")}
			if err := p.Publish(t.Context(), msg); err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			if got := headers(); got != nil {
				t.Errorf("headers = %v, want nil", got)
			}
		})
	}
}

// The transport receives a copy: a queue that mutates the map it keeps cannot
// corrupt the message the relay still holds for a retry.
func TestPublisher_headersAreCopied(t *testing.T) {
	t.Parallel()

	q := &stubQueue{}

	p, err := QueuePublisher(q)
	if err != nil {
		t.Fatalf("QueuePublisher failed: %v", err)
	}

	msg := sampleMessage()
	if err := p.Publish(t.Context(), msg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	q.pushes[0].Headers["injected-by-transport"] = "boom"

	if _, ok := msg.Headers["injected-by-transport"]; ok {
		t.Error("transport mutation leaked into the message headers")
	}

	if len(msg.Headers) != 1 {
		t.Errorf("message headers = %v, want the single original entry", msg.Headers)
	}
}

func TestPublisher_transportErrorWrapped(t *testing.T) {
	t.Parallel()

	errTransport := errors.New("boom")

	tests := []struct {
		name    string
		build   func(t *testing.T) (outbox.Publisher, error)
		publish func(outbox.Publisher) error
	}{
		{
			name: "queue",
			build: func(t *testing.T) (outbox.Publisher, error) {
				t.Helper()

				p, err := QueuePublisher(&stubQueue{err: errTransport})
				if err != nil {
					t.Fatalf("QueuePublisher failed: %v", err)
				}

				return p, nil
			},
			publish: func(p outbox.Publisher) error { return p.Publish(context.Background(), sampleMessage()) },
		},
		{
			name: "eventbus",
			build: func(t *testing.T) (outbox.Publisher, error) {
				t.Helper()

				p, err := EventBusPublisher(&stubEventBus{err: errTransport})
				if err != nil {
					t.Fatalf("EventBusPublisher failed: %v", err)
				}

				return p, nil
			},
			publish: func(p outbox.Publisher) error { return p.Publish(context.Background(), sampleMessage()) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, buildErr := tt.build(t)
			if buildErr != nil {
				t.Fatalf("build failed: %v", buildErr)
			}

			err := tt.publish(p)
			if err == nil {
				t.Fatal("Publish succeeded, want transport error")
			}

			// The relay branches on errors.Is, so the transport cause must
			// survive the wrap.
			if !errors.Is(err, errTransport) {
				t.Errorf("error %v does not wrap the transport cause", err)
			}
		})
	}
}

func TestPublisher_nilTransport(t *testing.T) {
	t.Parallel()

	if _, err := QueuePublisher(nil); !errors.Is(err, ErrNilQueue) {
		t.Errorf("QueuePublisher(nil) error = %v, want ErrNilQueue", err)
	}

	if _, err := EventBusPublisher(nil); !errors.Is(err, ErrNilEventBus) {
		t.Errorf("EventBusPublisher(nil) error = %v, want ErrNilEventBus", err)
	}
}

func TestKey_nilHeaders(t *testing.T) {
	t.Parallel()

	if got := Key(nil); got != "" {
		t.Errorf("Key(nil) = %q, want empty", got)
	}
}
