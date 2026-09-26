// Package queuetest provides the conformance kit third-party queue adapters run to prove backend parity.
package queuetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

const (
	// DefaultPollTimeout bounds how long Pop waits on an empty topic inside
	// conformance tests. Proof factories should configure a similarly short
	// PollTimeout so empty-queue cases stay fast.
	DefaultPollTimeout = 50 * time.Millisecond
	// DefaultDeliveryTimeout bounds how long delayed-delivery polls wait for
	// a message to become available before failing.
	DefaultDeliveryTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between delayed-delivery poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
	// DefaultDelayedDelay is the delay PushDelayed tests request before
	// polling for the message.
	DefaultDelayedDelay = 60 * time.Millisecond
)

// Conformance verifies factory-built queues implement the queue.Queue
// contract: FIFO enqueue/dequeue, empty-queue behavior, Length/IsEmpty,
// Ack, Nack requeue/drop, delayed delivery, per-topic isolation, and Close.
// Each subtest takes a fresh instance from factory so cases stay isolated.
// Delayed-delivery waits poll with a context deadline; they never
// synchronize with time.Sleep and never touch the network.
func Conformance(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	t.Run("FIFO", func(t *testing.T) { conformanceFIFO(t, factory) })
	t.Run("Empty", func(t *testing.T) { conformanceEmpty(t, factory) })
	t.Run("LengthIsEmpty", func(t *testing.T) { conformanceLengthIsEmpty(t, factory) })
	t.Run("Ack", func(t *testing.T) { conformanceAck(t, factory) })
	t.Run("NackRequeue", func(t *testing.T) { conformanceNackRequeue(t, factory) })
	t.Run("NackDrop", func(t *testing.T) { conformanceNackDrop(t, factory) })
	t.Run("Delayed", func(t *testing.T) { conformanceDelayed(t, factory) })
	t.Run("TopicsIsolated", func(t *testing.T) { conformanceTopicsIsolated(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceFIFO(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "fifo"

	payloads := []string{"first", "second", "third"}
	for i, p := range payloads {
		headers := queue.Headers(nil)
		if i == 1 {
			headers = queue.Headers{"k": "v"}
		}

		if err := q.Push(ctx, topic, queue.Payload(p), headers); err != nil {
			t.Fatalf("Push(%q) error = %v", p, err)
		}
	}

	seen := make(map[queue.MessageID]bool)

	for i, want := range payloads {
		msg, err := q.Pop(ctx, topic)
		if err != nil {
			t.Fatalf("Pop(%d) error = %v", i, err)
		}

		if msg.Topic != topic {
			t.Errorf("Pop(%d) Topic = %q, want %q", i, msg.Topic, topic)
		}

		if string(msg.Payload) != want {
			t.Errorf("Pop(%d) Payload = %q, want %q", i, msg.Payload, want)
		}

		if msg.Attempt != 1 {
			t.Errorf("Pop(%d) Attempt = %d, want 1", i, msg.Attempt)
		}

		if msg.ID == (queue.MessageID{}) {
			t.Errorf("Pop(%d) ID is zero", i)
		}

		if seen[msg.ID] {
			t.Errorf("Pop(%d) ID %v duplicated", i, msg.ID)
		}

		seen[msg.ID] = true

		if i == 1 && msg.Headers["k"] != "v" {
			t.Errorf("Pop(1) Headers[k] = %q, want v", msg.Headers["k"])
		}

		if err := q.Ack(ctx, msg); err != nil {
			t.Fatalf("Ack(%d) error = %v", i, err)
		}
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		t.Errorf("Length() = %d,%v want 0,nil", n, err)
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop() drained err = %v, want ErrEmpty", err)
	}
}

func conformanceEmpty(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	if _, err := q.Pop(ctx, "never-used-topic"); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop(missing topic) err = %v, want ErrEmpty", err)
	}

	const topic = "empty"

	if err := q.Push(ctx, topic, queue.Payload("only"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	// The message is inflight now, so the topic holds no ready message: the
	// backend must report empty with the topic attached.
	_, err = q.Pop(ctx, topic)
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop(inflight only) err = %v, want ErrEmpty", err)
	}

	var emptyErr *queue.EmptyError
	if !errors.As(err, &emptyErr) {
		t.Fatalf("errors.As(err, EmptyError) = false (err = %T %v)", err, err)
	}

	if emptyErr.Topic != topic {
		t.Errorf("EmptyError.Topic = %q, want %q", emptyErr.Topic, topic)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := q.Pop(cancelled, topic); err == nil {
		t.Error("Pop(cancelled ctx) = nil, want error")
	}
}

func conformanceLengthIsEmpty(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "length"

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		t.Fatalf("Length(missing) = %d,%v want 0,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		t.Fatalf("IsEmpty(missing) = %v,%v want true,nil", ok, err)
	}

	if err := q.Push(ctx, topic, queue.Payload("a"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if err := q.Push(ctx, topic, queue.Payload("b"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 2 {
		t.Fatalf("Length() = %d,%v want 2,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || ok {
		t.Fatalf("IsEmpty() = %v,%v want false,nil", ok, err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	// Inflight messages are not ready: Length counts ready messages only.
	if n, err := q.Length(ctx, topic); err != nil || n != 1 {
		t.Errorf("Length(inflight) = %d,%v want 1,nil", n, err)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 1 {
		t.Errorf("Length(after ack) = %d,%v want 1,nil", n, err)
	}
}

func conformanceAck(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "ack"

	if err := q.Push(ctx, topic, queue.Payload("work"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	// Ack is idempotent: acknowledging twice stays nil.
	if err := q.Ack(ctx, msg); err != nil {
		t.Errorf("Ack(again) error = %v, want nil", err)
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop(after ack) err = %v, want ErrEmpty", err)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		t.Errorf("Length(after ack) = %d,%v want 0,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		t.Errorf("IsEmpty(after ack) = %v,%v want true,nil", ok, err)
	}
}

func conformanceNackRequeue(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "nack-requeue"

	if err := q.Push(ctx, topic, queue.Payload("work"), queue.Headers{"k": "v"}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	first, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	// Mutate the popped copy: redelivery must observe the stored message,
	// not caller-side mutations.
	first.Payload[0] = 'X'
	first.Headers["k"] = "mutated"

	if nerr := q.Nack(ctx, first, true); nerr != nil {
		t.Fatalf("Nack(requeue) error = %v", nerr)
	}

	second, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop(redelivered) error = %v", err)
	}

	if string(second.Payload) != "work" {
		t.Errorf("redelivered Payload = %q, want work (stored copy)", second.Payload)
	}

	if second.Headers["k"] != "v" {
		t.Errorf("redelivered Headers[k] = %q, want v (stored copy)", second.Headers["k"])
	}

	if second.Attempt <= first.Attempt {
		t.Errorf("redelivered Attempt = %d, want > %d", second.Attempt, first.Attempt)
	}

	if err := q.Ack(ctx, second); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop(after ack) err = %v, want ErrEmpty", err)
	}
}

func conformanceNackDrop(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "nack-drop"

	if err := q.Push(ctx, topic, queue.Payload("work"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	if err := q.Nack(ctx, msg, false); err != nil {
		t.Fatalf("Nack(drop) error = %v", err)
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop(after drop) err = %v, want ErrEmpty", err)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		t.Errorf("Length(after drop) = %d,%v want 0,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		t.Errorf("IsEmpty(after drop) = %v,%v want true,nil", ok, err)
	}
}

func conformanceDelayed(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	const topic = "delayed"

	// Non-positive delays are immediately available.
	if err := q.PushDelayed(ctx, topic, queue.Payload("now"), nil, 0); err != nil {
		t.Fatalf("PushDelayed(0) error = %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop(immediate) error = %v", err)
	}

	if string(msg.Payload) != "now" {
		t.Fatalf("Pop(immediate) Payload = %q, want now", msg.Payload)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	if err := q.PushDelayed(ctx, topic, queue.Payload("later"), nil, DefaultDelayedDelay); err != nil {
		t.Fatalf("PushDelayed() error = %v", err)
	}

	// The delayed message must not be ready yet.
	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop(before delay) err = %v, want ErrEmpty", err)
	}

	eventually(t, "delayed message delivered", func(ctx context.Context) bool {
		got, err := q.Pop(ctx, topic)
		if err != nil {
			return false
		}

		if string(got.Payload) != "later" {
			t.Errorf("Pop(delayed) Payload = %q, want later", got.Payload)
			return true
		}

		return true
	})
}

func conformanceTopicsIsolated(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	if err := q.Push(ctx, "topic-a", queue.Payload("a"), nil); err != nil {
		t.Fatalf("Push(a) error = %v", err)
	}

	if err := q.Push(ctx, "topic-b", queue.Payload("b"), nil); err != nil {
		t.Fatalf("Push(b) error = %v", err)
	}

	msgB, err := q.Pop(ctx, "topic-b")
	if err != nil {
		t.Fatalf("Pop(b) error = %v", err)
	}

	if string(msgB.Payload) != "b" {
		t.Errorf("Pop(b) Payload = %q, want b", msgB.Payload)
	}

	msgA, err := q.Pop(ctx, "topic-a")
	if err != nil {
		t.Fatalf("Pop(a) error = %v", err)
	}

	if string(msgA.Payload) != "a" {
		t.Errorf("Pop(a) Payload = %q, want a", msgA.Payload)
	}

	if err := q.Ack(ctx, msgA); err != nil {
		t.Fatalf("Ack(a) error = %v", err)
	}

	if err := q.Ack(ctx, msgB); err != nil {
		t.Fatalf("Ack(b) error = %v", err)
	}

	if ok, err := q.IsEmpty(ctx, "topic-a"); err != nil || !ok {
		t.Errorf("IsEmpty(a) = %v,%v want true,nil", ok, err)
	}

	if ok, err := q.IsEmpty(ctx, "topic-b"); err != nil || !ok {
		t.Errorf("IsEmpty(b) = %v,%v want true,nil", ok, err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	ctx := t.Context()
	q := factory(t)

	if q.Name() == "" {
		t.Error("Name() is empty, want adapter name")
	}

	if err := q.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := q.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if err := q.Push(ctx, "closed", queue.Payload("x"), nil); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Push() err = %v, want ErrClosed", err)
	}

	if err := q.PushDelayed(ctx, "closed", queue.Payload("x"), nil, 0); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("PushDelayed() err = %v, want ErrClosed", err)
	}
}

// eventually polls cond until true or DefaultDeliveryTimeout elapses. Poll
// ticks use a ticker, never time.Sleep, and cond receives a deadline-bound
// context so backend calls share the same deadline.
func eventually(t *testing.T, msg string, cond func(ctx context.Context) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultDeliveryTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met within %v: %s", DefaultDeliveryTimeout, msg)
		case <-ticker.C:
		}
	}
}
