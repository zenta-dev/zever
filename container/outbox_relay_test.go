package container

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/outboxbridge"
)

// relayPush is one message the relay delivered to a transport.
type relayPush struct {
	Topic   string
	Payload []byte
	Headers map[string]string
}

// stubRelayQueue records the relay's pushes. It is a test double for
// queue.Queue: only Push carries relay traffic, the rest satisfy the interface.
type stubRelayQueue struct {
	mu     sync.Mutex
	pushes []relayPush
}

func (q *stubRelayQueue) Push(_ context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.pushes = append(q.pushes, relayPush{Topic: topic, Payload: payload, Headers: headers})

	return nil
}

func (q *stubRelayQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (q *stubRelayQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (q *stubRelayQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (q *stubRelayQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (q *stubRelayQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (q *stubRelayQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (q *stubRelayQueue) Close() error { return nil }

func (q *stubRelayQueue) Name() string { return "stub-relay" }

func (q *stubRelayQueue) recorded() []relayPush {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]relayPush, len(q.pushes))
	copy(out, q.pushes)

	return out
}

// stubRelayBus records the relay's publishes.
type stubRelayBus struct {
	mu     sync.Mutex
	pushes []relayPush
}

func (b *stubRelayBus) Publish(_ context.Context, topic string, payload eventbus.Payload, headers eventbus.Headers) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pushes = append(b.pushes, relayPush{Topic: topic, Payload: payload, Headers: headers})

	return nil
}

func (b *stubRelayBus) Subscribe(_ context.Context, _ string, _ eventbus.Handler) (func(), error) {
	return func() {}, nil
}

func (b *stubRelayBus) SubscribeChan(_ context.Context, _ string, _ int) (<-chan eventbus.Message, error) {
	ch := make(chan eventbus.Message)

	return ch, nil
}

func (b *stubRelayBus) Unsubscribe(_ string, _ <-chan eventbus.Message) error { return nil }

func (b *stubRelayBus) Close() error { return nil }

func (b *stubRelayBus) Name() string { return "stub-relay" }

func (b *stubRelayBus) recorded() []relayPush {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]relayPush, len(b.pushes))
	copy(out, b.pushes)

	return out
}

// relayMessage is the single message the stub relay publishes on Start.
func relayMessage() outbox.Message {
	return outbox.Message{ID: "relay-1", Topic: "orders.created", Key: "tenant-7", Payload: []byte("body")}
}

// stubRelayStore is an outbox.Store that accepts a publisher after Open and
// publishes one message from Start, so the whole attach-then-start chain is
// exercised without a database, a broker, or a timer.
type stubRelayStore struct {
	publisher outbox.Publisher
	startErr  error
	started   bool
	closed    bool
}

func (s *stubRelayStore) Record(_ context.Context, _ db.Tx, _ outbox.Message) error { return nil }

// Start publishes one message through the attached publisher: no goroutine, no
// polling, so the test stays deterministic.
func (s *stubRelayStore) Start(ctx context.Context) error {
	if s.startErr != nil {
		return s.startErr
	}

	if s.publisher == nil {
		return outbox.ErrInvalidOptions
	}

	s.started = true

	return s.publisher.Publish(ctx, relayMessage())
}

func (s *stubRelayStore) Status() outbox.Status { return outbox.Status{} }

func (s *stubRelayStore) Close() error {
	s.closed = true

	return nil
}

func (s *stubRelayStore) Name() string { return "stub-relay" }

func (s *stubRelayStore) SetPublisher(p outbox.Publisher) {
	if p == nil {
		return
	}

	s.publisher = p
}

// stubStoreNoSetter hides the SetPublisher method by embedding the interface,
// standing in for a store whose relay cannot be attached (the in-memory
// adapter publishes from Record instead).
type stubStoreNoSetter struct {
	outbox.Store
}

// mustSeed stores value in the container's lazy slot, the only same-package
// seam the lazy contract exposes. Seeding skips the adapter registry so no real
// database or broker is opened.
func mustSeed[T any](t *testing.T, slot *lazy[T], value T) {
	t.Helper()

	if _, err := slot.get(func() (T, error) { return value, nil }); err != nil {
		t.Fatalf("seeding lazy slot failed: %v", err)
	}
}

// closeRelayContainer shuts the container down with a live context.
// closeContainer passes t.Context(), which the testing package cancels before
// cleanups run, so every close would race the timeout branch and report a
// phantom "close timed out". Relay tests care about the close actually
// happening, so they own their shutdown.
func closeRelayContainer(t *testing.T, c *Container) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := c.Close(ctx); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
}

var (
	_ queue.Queue            = (*stubRelayQueue)(nil)
	_ eventbus.EventBus      = (*stubRelayBus)(nil)
	_ outbox.Store           = (*stubRelayStore)(nil)
	_ outbox.PublisherSetter = (*stubRelayStore)(nil)
	_ outbox.Store           = (*stubStoreNoSetter)(nil)
)

// relayConfig returns a config whose outbox transport selector is publisher.
func relayConfig(t *testing.T, publisher string) *config.Config {
	t.Helper()

	cfg := testConfig(t)
	cfg.Outbox.Options.Publisher = publisher

	return cfg
}

// TestOutboxRelay_transportSelector pins the selector semantics: "eventbus"
// publishes through the bus, "queue" and the empty default through the queue.
func TestOutboxRelay_transportSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		publisher string
		wantQueue bool
		wantBus   bool
	}{
		{name: "default", publisher: "", wantQueue: true},
		{name: "queue", publisher: outbox.PublisherQueue, wantQueue: true},
		{name: "eventbus", publisher: outbox.PublisherEventBus, wantBus: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(relayConfig(t, tt.publisher))
			closeRelayContainer(t, c)

			store := &stubRelayStore{}
			q := &stubRelayQueue{}
			bus := &stubRelayBus{}

			mustSeed(t, &c.outbox, outbox.Store(store))
			mustSeed(t, &c.queue, queue.Queue(q))
			mustSeed(t, &c.eventbus, eventbus.EventBus(bus))

			started, err := c.OutboxRelay(t.Context())
			if err != nil {
				t.Fatalf("OutboxRelay() error = %v", err)
			}

			if started != outbox.Store(store) {
				t.Fatalf("OutboxRelay() = %T, want the cached outbox store", started)
			}

			if !store.started {
				t.Fatal("OutboxRelay() did not start the relay")
			}

			// The attached publisher carries the whole message, including the
			// routing key the queue API cannot express.
			if tt.wantQueue {
				got := q.recorded()
				if len(got) != 1 {
					t.Fatalf("queue recorded %d pushes, want 1", len(got))
				}

				if got[0].Topic != relayMessage().Topic {
					t.Errorf("queue topic = %q, want %q", got[0].Topic, relayMessage().Topic)
				}

				if string(got[0].Payload) != string(relayMessage().Payload) {
					t.Errorf("queue payload = %q, want %q", got[0].Payload, relayMessage().Payload)
				}

				if key := outboxbridge.Key(got[0].Headers); key != relayMessage().Key {
					t.Errorf("queue %s = %q, want %q", outboxbridge.KeyHeader, key, relayMessage().Key)
				}
			}

			if tt.wantBus {
				got := bus.recorded()
				if len(got) != 1 {
					t.Fatalf("eventbus recorded %d publishes, want 1", len(got))
				}

				if got[0].Topic != relayMessage().Topic {
					t.Errorf("eventbus topic = %q, want %q", got[0].Topic, relayMessage().Topic)
				}
			}

			if n := len(bus.recorded()); tt.wantQueue && n != 0 {
				t.Errorf("eventbus recorded %d publishes, want 0 for a queue relay", n)
			}

			if n := len(q.recorded()); tt.wantBus && n != 0 {
				t.Errorf("queue recorded %d pushes, want 0 for an eventbus relay", n)
			}
		})
	}
}

// The relay publishes into the process-wide queue instance, the same reference
// Job() hands out, so a worker popping from it sees the relay's messages.
func TestOutboxRelay_sharesResolvedTransport(t *testing.T) {
	t.Parallel()

	c := New(relayConfig(t, ""))
	closeRelayContainer(t, c)

	store := &stubRelayStore{}
	q := &stubRelayQueue{}

	mustSeed(t, &c.outbox, outbox.Store(store))
	mustSeed(t, &c.queue, queue.Queue(q))

	if _, err := c.OutboxRelay(t.Context()); err != nil {
		t.Fatalf("OutboxRelay() error = %v", err)
	}

	resolved, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue() error = %v", err)
	}

	if resolved != queue.Queue(q) {
		t.Error("OutboxRelay used a different queue instance than Queue() resolves")
	}
}

// The store OutboxRelay starts is the cached Outbox() instance, so Record keeps
// writing to the table the relay drains and Close shuts the relay down.
func TestOutboxRelay_reusesCachedOutboxStore(t *testing.T) {
	t.Parallel()

	c := New(relayConfig(t, ""))
	closeRelayContainer(t, c)

	store := &stubRelayStore{}

	mustSeed(t, &c.outbox, outbox.Store(store))
	mustSeed(t, &c.queue, queue.Queue(&stubRelayQueue{}))

	started, err := c.OutboxRelay(t.Context())
	if err != nil {
		t.Fatalf("OutboxRelay() error = %v", err)
	}

	resolved, err := c.Outbox()
	if err != nil {
		t.Fatalf("Outbox() error = %v", err)
	}

	sameInstance(t, started, resolved)

	again, err := c.OutboxRelay(t.Context())
	if err != nil {
		t.Fatalf("second OutboxRelay() error = %v, want nil (Start is idempotent)", err)
	}

	if again != started {
		t.Error("second OutboxRelay() built a new store")
	}
}

// An unresolvable selected transport is a boot error: a silently dead relay
// loses business events while looking healthy.
func TestOutboxRelay_transportFailureIsBootError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		publisher string
		breakIt   func(cfg *config.Config)
		want      string
	}{
		{
			name:      "queue",
			publisher: "",
			breakIt:   func(cfg *config.Config) { cfg.Queue.Adapter = "bogus-adapter" },
			want:      "queue",
		},
		{
			name:      "eventbus",
			publisher: outbox.PublisherEventBus,
			breakIt:   func(cfg *config.Config) { cfg.EventBus.Adapter = "bogus-adapter" },
			want:      "eventbus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := relayConfig(t, tt.publisher)
			tt.breakIt(cfg)

			c := New(cfg)
			closeRelayContainer(t, c)

			store := &stubRelayStore{}
			mustSeed(t, &c.outbox, outbox.Store(store))

			got, err := c.OutboxRelay(t.Context())
			if err == nil {
				t.Fatal("OutboxRelay() = nil error, want a boot error")
			}

			if got != nil {
				t.Errorf("OutboxRelay() returned a store alongside the error: %T", got)
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should name the unresolved transport %q", err, tt.want)
			}

			if store.started {
				t.Error("relay started without a transport")
			}
		})
	}
}

// A store that cannot take a publisher is rejected by name instead of being
// started with nowhere to publish to.
func TestOutboxRelay_storeWithoutPublisherSetter(t *testing.T) {
	t.Parallel()

	c := New(relayConfig(t, ""))
	closeRelayContainer(t, c)

	mustSeed(t, &c.outbox, outbox.Store(stubStoreNoSetter{Store: &stubRelayStore{}}))
	mustSeed(t, &c.queue, queue.Queue(&stubRelayQueue{}))

	_, err := c.OutboxRelay(t.Context())
	if err == nil {
		t.Fatal("OutboxRelay() = nil error, want unsupported-store error")
	}

	var perr PublisherSetterError
	if !errors.As(err, &perr) {
		t.Fatalf("error is %T, want PublisherSetterError", err)
	}

	if !strings.Contains(perr.Actual, "stubStoreNoSetter") {
		t.Errorf("PublisherSetterError.Actual = %q, want it to name stubStoreNoSetter", perr.Actual)
	}

	if !errors.Is(err, ErrPublisherSetterUnsupported) {
		t.Errorf("error should unwrap to ErrPublisherSetterUnsupported, got: %v", err)
	}
}

func TestOutboxRelay_startFailurePropagates(t *testing.T) {
	t.Parallel()

	c := New(relayConfig(t, ""))
	closeRelayContainer(t, c)

	errStart := errors.New("relay refused to start")

	mustSeed(t, &c.outbox, outbox.Store(&stubRelayStore{startErr: errStart}))
	mustSeed(t, &c.queue, queue.Queue(&stubRelayQueue{}))

	_, err := c.OutboxRelay(t.Context())
	if err == nil {
		t.Fatal("OutboxRelay() = nil error, want the adapter's start failure")
	}

	if !errors.Is(err, errStart) {
		t.Errorf("error %v should wrap the adapter failure", err)
	}

	if !strings.Contains(err.Error(), "start") {
		t.Errorf("error %q should name the failing step", err)
	}
}
