// Package outboxtest provides the conformance kit every outbox adapter runs
// to prove backend parity: Record→publish, retry on publisher error, DLQ
// after MaxAttempts, and Inbox dedupe.
package outboxtest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

const (
	// DefaultDeliveryTimeout bounds how long the kit waits for an async
	// relay to publish before failing.
	DefaultDeliveryTimeout = 3 * time.Second
	// DefaultPollInterval is the tick between delivery checks.
	DefaultPollInterval = 5 * time.Millisecond
)

// errPublish is the injected publisher failure used by retry/DLQ subtests.
var errPublish = errors.New("outboxtest: publish failed")

// Harness adapts one Store instance to the kit. Durable and Inbox describe
// the adapter's capabilities; subtests for absent capabilities are skipped.
type Harness struct {
	// Store is the adapter under test.
	Store outbox.Store
	// InTx runs fn in a transaction. DB-backed adapters begin/commit a real
	// transaction; non-transactional adapters (memory) pass a nil tx.
	// Required.
	InTx func(ctx context.Context, fn func(ctx context.Context, tx db.Tx) error) error
	// Inbox, when non-nil, enables the dedupe subtest.
	Inbox outbox.Inbox
	// Durable enables the retry and DLQ subtests. Dev adapters leave it false.
	Durable bool
}

// Recorder is a Publisher that records delivered messages and can be made to
// fail, so the kit can drive retry and DLQ behavior. It is safe for
// concurrent use.
type Recorder struct {
	mu        sync.Mutex
	msgs      []outbox.Message
	fail      func(outbox.Message) error
	remaining int
	notify    chan struct{}
}

// NewRecorder returns an empty Recorder.
func NewRecorder() *Recorder {
	return &Recorder{notify: make(chan struct{}, 1)}
}

// Publish records msg unless a failure is currently injected.
func (r *Recorder) Publish(_ context.Context, msg outbox.Message) error {
	r.mu.Lock()

	if r.fail != nil {
		if err := r.fail(msg); err != nil {
			r.mu.Unlock()

			return err
		}
	}

	r.msgs = append(r.msgs, msg.Clone())
	r.mu.Unlock()

	select {
	case r.notify <- struct{}{}:
	default:
	}

	return nil
}

// Messages returns a copy of the delivered messages.
func (r *Recorder) Messages() []outbox.Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]outbox.Message, len(r.msgs))
	copy(out, r.msgs)

	return out
}

// FailAlways injects a persistent publish failure.
func (r *Recorder) FailAlways() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remaining = 0
	r.fail = func(outbox.Message) error { return errPublish }
}

// FailFirst injects a failure for the next n publish calls, then succeeds.
func (r *Recorder) FailFirst(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remaining = n
	r.fail = func(outbox.Message) error {
		if r.remaining > 0 {
			r.remaining--

			return errPublish
		}

		return nil
	}
}

// ClearFail removes any injected failure.
func (r *Recorder) ClearFail() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fail = nil
	r.remaining = 0
}

// Conformance runs the outbox adapter conformance suite. factory must return
// a fresh Harness per subtest, wired to pub as its Publisher. Durable
// adapters should configure a short PollInterval and backoff so retry and DLQ
// cases stay fast.
func Conformance(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	t.Run("RecordPublish", func(t *testing.T) { conformanceRecordPublish(t, factory) })
	t.Run("Retry", func(t *testing.T) { conformanceRetry(t, factory) })
	t.Run("DLQ", func(t *testing.T) { conformanceDLQ(t, factory) })
	t.Run("InboxDedupe", func(t *testing.T) { conformanceInboxDedupe(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceRecordPublish(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	ctx := t.Context()
	pub := NewRecorder()
	h := factory(t, pub)

	t.Cleanup(func() { _ = h.Store.Close() })

	if err := h.Store.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	msg := outbox.Message{
		ID:      "conf-1",
		Topic:   "orders",
		Key:     "tenant-1",
		Payload: []byte("payload"),
		Headers: map[string]string{"k": "v"},
	}

	if err := record(ctx, h, msg); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	got := eventually(t, pub, 1)

	if got[0].ID != msg.ID {
		t.Errorf("published ID = %q, want %q", got[0].ID, msg.ID)
	}

	if got[0].Topic != msg.Topic {
		t.Errorf("published Topic = %q, want %q", got[0].Topic, msg.Topic)
	}

	if got[0].Key != msg.Key {
		t.Errorf("published Key = %q, want %q", got[0].Key, msg.Key)
	}

	if string(got[0].Payload) != string(msg.Payload) {
		t.Errorf("published Payload = %q, want %q", got[0].Payload, msg.Payload)
	}

	if got[0].Headers["k"] != "v" {
		t.Errorf("published Headers[k] = %q, want v", got[0].Headers["k"])
	}
}

func conformanceRetry(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	ctx := t.Context()
	pub := NewRecorder()
	h := factory(t, pub)

	t.Cleanup(func() { _ = h.Store.Close() })

	if !h.Durable {
		t.Skip("adapter is not durable; retry not applicable")
	}

	pub.FailFirst(1)

	if err := h.Store.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := record(ctx, h, outbox.Message{ID: "conf-retry", Topic: "orders", Payload: []byte("x")}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	got := eventually(t, pub, 1)

	if got[0].Attempts < 2 {
		t.Errorf("published Attempts = %d, want >= 2 (a retry)", got[0].Attempts)
	}

	if st := h.Store.Status(); st.Failed != 0 {
		t.Errorf("Status().Failed = %d, want 0", st.Failed)
	}
}

func conformanceDLQ(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	ctx := t.Context()
	pub := NewRecorder()
	h := factory(t, pub)

	t.Cleanup(func() { _ = h.Store.Close() })

	if !h.Durable {
		t.Skip("adapter is not durable; DLQ not applicable")
	}

	pub.FailAlways()

	if err := h.Store.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := record(ctx, h, outbox.Message{ID: "conf-dlq", Topic: "orders", Payload: []byte("x")}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	eventuallyStatus(t, h.Store, func(st outbox.Status) bool { return st.Failed >= 1 })

	st := h.Store.Status()
	if st.Pending != 0 {
		t.Errorf("Status().Pending = %d, want 0 after DLQ", st.Pending)
	}

	if st.LastError == "" {
		t.Error("Status().LastError is empty, want the publish error")
	}
}

func conformanceInboxDedupe(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	ctx := t.Context()
	pub := NewRecorder()
	h := factory(t, pub)

	t.Cleanup(func() { _ = h.Store.Close() })

	if h.Inbox == nil {
		t.Skip("adapter has no inbox")
	}

	calls := 0

	var fnErr error

	fn := func(context.Context, db.Tx) error {
		calls++

		return fnErr
	}

	process := func(eventID string) error {
		return h.InTx(ctx, func(ctx context.Context, tx db.Tx) error {
			return h.Inbox.Process(ctx, tx, eventID, fn)
		})
	}

	if err := process("evt-1"); err != nil {
		t.Fatalf("Process(first) error = %v", err)
	}

	if err := process("evt-1"); err != nil {
		t.Fatalf("Process(repeat) error = %v", err)
	}

	if calls != 1 {
		t.Errorf("fn calls = %d, want 1 (deduped)", calls)
	}

	// A failing side effect must propagate out of Process.
	fnErr = errPublish

	if err := process("evt-2"); !errors.Is(err, errPublish) {
		t.Errorf("Process(failing fn) = %v, want errPublish", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T, pub *Recorder) Harness) {
	t.Helper()

	ctx := t.Context()
	pub := NewRecorder()
	h := factory(t, pub)

	if h.Store.Name() == "" {
		t.Error("Name() is empty, want adapter name")
	}

	if err := h.Store.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := h.Store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := h.Store.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}

// record persists msg through the harness transaction adapter.
func record(ctx context.Context, h Harness, msg outbox.Message) error {
	return h.InTx(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.Store.Record(ctx, tx, msg)
	})
}

// eventually waits until pub has at least n messages and returns them.
func eventually(t *testing.T, pub *Recorder, n int) []outbox.Message {
	t.Helper()

	eventuallyStatus(t, nil, func(outbox.Status) bool { return len(pub.Messages()) >= n })

	return pub.Messages()
}

// eventuallyStatus polls cond until true or DefaultDeliveryTimeout elapses.
// When store is non-nil its status is passed to cond; otherwise a zero Status
// is passed. It ticks on a timer, never time.Sleep.
func eventuallyStatus(t *testing.T, store outbox.Store, cond func(outbox.Status) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultDeliveryTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		var st outbox.Status
		if store != nil {
			st = store.Status()
		}

		if cond(st) {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met within %v", DefaultDeliveryTimeout)
		case <-ticker.C:
		}
	}
}
