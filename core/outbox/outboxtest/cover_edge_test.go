package outboxtest_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

// stubDurable is a durable Store that retries Publish synchronously inside
// Record, mirroring a polling relay with immediate delivery. It is safe for
// concurrent use.
type stubDurable struct {
	mu          sync.Mutex
	pub         outbox.Publisher
	maxAttempts int
	processed   int64
	failed      int64
	lastErr     string
}

func (s *stubDurable) Record(ctx context.Context, _ db.Tx, msg outbox.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	for attempt := 1; ; attempt++ {
		m := msg.Clone()
		m.Attempts = attempt

		err := s.pub.Publish(ctx, m)
		if err == nil {
			s.mu.Lock()
			s.processed++
			s.mu.Unlock()

			return nil
		}

		s.mu.Lock()
		s.lastErr = err.Error()
		exhausted := attempt >= s.maxAttempts

		if exhausted {
			s.failed++
		}

		s.mu.Unlock()

		if exhausted {
			return nil
		}
	}
}

func (s *stubDurable) Start(context.Context) error { return nil }

func (s *stubDurable) Status() outbox.Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return outbox.Status{Processed: s.processed, Failed: s.failed, LastError: s.lastErr}
}

func (s *stubDurable) Close() error { return nil }

func (s *stubDurable) Name() string { return "durable-stub" }

// stubInbox is an idempotent Inbox backed by a dedupe set. It is safe for
// concurrent use.
type stubInbox struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func (s *stubInbox) Process(_ context.Context, _ db.Tx, eventID string, fn func(context.Context, db.Tx) error) error {
	s.mu.Lock()

	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}

	if _, ok := s.seen[eventID]; ok {
		s.mu.Unlock()

		return nil
	}

	s.seen[eventID] = struct{}{}
	s.mu.Unlock()

	return fn(context.Background(), nil)
}

func passthroughTx(ctx context.Context, fn func(context.Context, db.Tx) error) error {
	return fn(ctx, nil)
}

func TestConformanceDurable(t *testing.T) {
	t.Parallel()

	outboxtest.Conformance(t, func(_ *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		return outboxtest.Harness{
			Store:   &stubDurable{pub: pub, maxAttempts: 5},
			InTx:    passthroughTx,
			Inbox:   &stubInbox{},
			Durable: true,
		}
	})
}

func TestRecorderFailAlways(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()
	r.FailAlways()

	err := r.Publish(t.Context(), outbox.Message{ID: "m-1", Topic: "orders"})
	if err == nil {
		t.Fatal("Publish(fail) = nil error, want error")
	}

	if !strings.Contains(err.Error(), "outboxtest: publish failed") {
		t.Errorf("Publish() error = %v, want publish failure", err)
	}

	if got := len(r.Messages()); got != 0 {
		t.Errorf("Messages() = %d, want 0 while failing", got)
	}

	r.ClearFail()

	if err := r.Publish(t.Context(), outbox.Message{ID: "m-1", Topic: "orders"}); err != nil {
		t.Fatalf("Publish(cleared) error = %v", err)
	}

	if got := len(r.Messages()); got != 1 {
		t.Errorf("Messages() = %d, want 1 after ClearFail", got)
	}
}

func TestRecorderFailFirst(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()
	r.FailFirst(2)

	msg := outbox.Message{ID: "m-1", Topic: "orders"}

	if err := r.Publish(t.Context(), msg); err == nil {
		t.Error("Publish(first) = nil error, want error")
	}

	if err := r.Publish(t.Context(), msg); err == nil {
		t.Error("Publish(second) = nil error, want error")
	}

	if err := r.Publish(t.Context(), msg); err != nil {
		t.Errorf("Publish(third) error = %v, want nil", err)
	}

	if got := len(r.Messages()); got != 1 {
		t.Errorf("Messages() = %d, want 1", got)
	}
}

func TestRecorderFailFirstZeroSucceeds(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()
	r.FailFirst(0)

	if err := r.Publish(t.Context(), outbox.Message{ID: "m-1", Topic: "orders"}); err != nil {
		t.Errorf("Publish(FailFirst(0)) error = %v, want nil", err)
	}
}

func TestRecorderMessagesCopy(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()
	msg := outbox.Message{ID: "m-1", Topic: "orders", Payload: []byte("p")}

	if err := r.Publish(t.Context(), msg); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	msg.Payload[0] = 'X'

	got := r.Messages()
	if len(got) != 1 || got[0].ID != "m-1" {
		t.Fatalf("Messages() = %+v, want one m-1", got)
	}

	if string(got[0].Payload) != "p" {
		t.Errorf("Messages()[0].Payload = %q, want p (cloned on Publish)", got[0].Payload)
	}

	got[0].ID = "mutated"

	fresh := r.Messages()
	if fresh[0].ID != "m-1" {
		t.Errorf("Messages()[0].ID = %q after mutation, want m-1 (copy)", fresh[0].ID)
	}
}

func TestRecorderNotifyDoesNotBlock(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()

	for i := range 3 {
		msg := outbox.Message{ID: "m", Topic: "orders"}
		msg.ID = "m-" + string(rune('1'+i))

		if err := r.Publish(t.Context(), msg); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}

	if got := len(r.Messages()); got != 3 {
		t.Errorf("Messages() = %d, want 3", got)
	}
}

func TestRecorderConcurrentUse(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			msg := outbox.Message{ID: "m", Topic: "orders"}
			_ = r.Publish(context.Background(), msg)
			_ = r.Messages()
		}()
	}

	wg.Wait()

	if got := len(r.Messages()); got != 8 {
		t.Errorf("Messages() = %d, want 8", got)
	}
}

func TestDurableStubEdge(t *testing.T) {
	t.Parallel()

	r := outboxtest.NewRecorder()
	s := &stubDurable{pub: r, maxAttempts: 2}

	if err := s.Record(t.Context(), nil, outbox.Message{}); err == nil {
		t.Error("Record(invalid) = nil error, want error")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := s.Record(ctx, nil, outbox.Message{ID: "m-1", Topic: "orders"}); err == nil {
		t.Error("Record(cancelled) = nil error, want error")
	}

	if s.Name() == "" {
		t.Error("Name() is empty, want adapter name")
	}

	if err := s.Start(t.Context()); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestDurableStubDefaults(t *testing.T) {
	t.Parallel()

	if outboxtest.DefaultDeliveryTimeout <= 0 {
		t.Errorf("DefaultDeliveryTimeout = %v, want positive", outboxtest.DefaultDeliveryTimeout)
	}

	if outboxtest.DefaultPollInterval <= 0 {
		t.Errorf("DefaultPollInterval = %v, want positive", outboxtest.DefaultPollInterval)
	}
}
