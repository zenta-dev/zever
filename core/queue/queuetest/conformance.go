// Package queuetest provides the conformance kit third-party queue adapters run to prove backend parity.
package queuetest

import (
	"context"
	"errors"
	"fmt"
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
// errf builds a non-wrapping descriptive error with Sprintf semantics.
// Check helpers use it (instead of fmt.Errorf with %w) for diagnostics
// where the formatted error may be nil: %w of a nil error prints
// "%!w(<nil>)", diverging from the historical Fatalf text, and errorlint
// forbids %v of an error in Errorf. Failure text stays byte-identical.
// It deliberately avoids fmt.Errorf so only real failures wrap.
func errf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)

	return errors.New(msg)
}

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

	if err := checkFIFO(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkFIFO proves FIFO order, payload/header/attempt/ID shape, Ack
// consumption, and drain-to-empty. Soft shape mismatches join; hard
// Push/Pop/Ack failures abort at the first one.
func checkFIFO(ctx context.Context, q queue.Queue) error {
	const topic = "fifo"

	payloads := []string{"first", "second", "third"}
	for i, p := range payloads {
		headers := queue.Headers(nil)
		if i == 1 {
			headers = queue.Headers{"k": "v"}
		}

		if err := q.Push(ctx, topic, queue.Payload(p), headers); err != nil {
			return fmt.Errorf("queuetest: Push(%q) error = %w", p, err)
		}
	}

	seen := make(map[queue.MessageID]bool)

	for i, want := range payloads {
		msg, err := q.Pop(ctx, topic)
		if err != nil {
			return fmt.Errorf("queuetest: Pop(%d) error = %w", i, err)
		}

		var errs []error

		if msg.Topic != topic {
			errs = append(errs, fmt.Errorf("queuetest: Pop(%d) Topic = %q, want %q", i, msg.Topic, topic))
		}

		if string(msg.Payload) != want {
			errs = append(errs, fmt.Errorf("queuetest: Pop(%d) Payload = %q, want %q", i, msg.Payload, want))
		}

		if msg.Attempt != 1 {
			errs = append(errs, fmt.Errorf("queuetest: Pop(%d) Attempt = %d, want 1", i, msg.Attempt))
		}

		if msg.ID == (queue.MessageID{}) {
			errs = append(errs, fmt.Errorf("queuetest: Pop(%d) ID is zero", i))
		}

		if seen[msg.ID] {
			errs = append(errs, fmt.Errorf("queuetest: Pop(%d) ID %v duplicated", i, msg.ID))
		}

		seen[msg.ID] = true

		if i == 1 && msg.Headers["k"] != "v" {
			errs = append(errs, fmt.Errorf("queuetest: Pop(1) Headers[k] = %q, want v", msg.Headers["k"]))
		}

		if err := errors.Join(errs...); err != nil {
			return err
		}

		if err := q.Ack(ctx, msg); err != nil {
			return fmt.Errorf("queuetest: Ack(%d) error = %w", i, err)
		}
	}

	var errs []error

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		errs = append(errs, errf("Length() = %d,%v want 0,nil", n, err))
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		errs = append(errs, errf("Pop() drained err = %v, want ErrEmpty", err))
	}

	return errors.Join(errs...)
}

func conformanceEmpty(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkEmpty(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkEmpty proves Pop on a missing topic and on an inflight-only topic
// reports ErrEmpty (with the topic attached via EmptyError), and that a
// cancelled context fails.
func checkEmpty(ctx context.Context, q queue.Queue) error {
	if _, err := q.Pop(ctx, "never-used-topic"); !errors.Is(err, queue.ErrEmpty) {
		return errf("Pop(missing topic) err = %v, want ErrEmpty", err)
	}

	const topic = "empty"

	if err := q.Push(ctx, topic, queue.Payload("only"), nil); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop() error = %w", err)
	}

	// The message is inflight now, so the topic holds no ready message: the
	// backend must report empty with the topic attached.
	_, err = q.Pop(ctx, topic)
	if !errors.Is(err, queue.ErrEmpty) {
		return errf("Pop(inflight only) err = %v, want ErrEmpty", err)
	}

	var emptyErr queue.EmptyError
	if !errors.As(err, &emptyErr) {
		return errf("errors.As(err, EmptyError) = false (err = %T %v)", err, err)
	}

	if emptyErr.Topic != topic {
		return fmt.Errorf("queuetest: EmptyError.Topic = %q, want %q", emptyErr.Topic, topic)
	}

	if err := q.Ack(ctx, msg); err != nil {
		return fmt.Errorf("queuetest: Ack() error = %w", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := q.Pop(cancelled, topic); err == nil {
		return errors.New("queuetest: Pop(cancelled ctx) = nil, want error")
	}

	return nil
}

func conformanceLengthIsEmpty(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkLengthIsEmpty(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkLengthIsEmpty proves Length/IsEmpty track ready messages only:
// inflight messages are excluded.
func checkLengthIsEmpty(ctx context.Context, q queue.Queue) error {
	const topic = "length"

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		return errf("Length(missing) = %d,%v want 0,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		return errf("IsEmpty(missing) = %v,%v want true,nil", ok, err)
	}

	if err := q.Push(ctx, topic, queue.Payload("a"), nil); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	if err := q.Push(ctx, topic, queue.Payload("b"), nil); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 2 {
		return errf("Length() = %d,%v want 2,nil", n, err)
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || ok {
		return errf("IsEmpty() = %v,%v want false,nil", ok, err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop() error = %w", err)
	}

	var errs []error

	// Inflight messages are not ready: Length counts ready messages only.
	if n, err := q.Length(ctx, topic); err != nil || n != 1 {
		errs = append(errs, errf("Length(inflight) = %d,%v want 1,nil", n, err))
	}

	if err := q.Ack(ctx, msg); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Ack() error = %w", err))
		return errors.Join(errs...)
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 1 {
		errs = append(errs, errf("Length(after ack) = %d,%v want 1,nil", n, err))
	}

	return errors.Join(errs...)
}

func conformanceAck(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkAck(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkAck proves Ack removes the message and is idempotent: a second
// Ack stays nil and the topic drains to empty.
func checkAck(ctx context.Context, q queue.Queue) error {
	const topic = "ack"

	if err := q.Push(ctx, topic, queue.Payload("work"), nil); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop() error = %w", err)
	}

	if err := q.Ack(ctx, msg); err != nil {
		return fmt.Errorf("queuetest: Ack() error = %w", err)
	}

	var errs []error

	// Ack is idempotent: acknowledging twice stays nil.
	if err := q.Ack(ctx, msg); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Ack(again) error = %w, want nil", err))
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		errs = append(errs, errf("Pop(after ack) err = %v, want ErrEmpty", err))
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		errs = append(errs, errf("Length(after ack) = %d,%v want 0,nil", n, err))
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		errs = append(errs, errf("IsEmpty(after ack) = %v,%v want true,nil", ok, err))
	}

	return errors.Join(errs...)
}

func conformanceNackRequeue(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkNackRequeue(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkNackRequeue proves Nack(requeue) redelivers the stored message
// (caller-side mutations invisible) with a bumped attempt count.
func checkNackRequeue(ctx context.Context, q queue.Queue) error {
	const topic = "nack-requeue"

	if err := q.Push(ctx, topic, queue.Payload("work"), queue.Headers{"k": "v"}); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	first, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop() error = %w", err)
	}

	// Mutate the popped copy: redelivery must observe the stored message,
	// not caller-side mutations.
	first.Payload[0] = 'X'
	first.Headers["k"] = "mutated"

	if nerr := q.Nack(ctx, first, true); nerr != nil {
		return fmt.Errorf("queuetest: Nack(requeue) error = %w", nerr)
	}

	second, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop(redelivered) error = %w", err)
	}

	var errs []error

	if string(second.Payload) != "work" {
		errs = append(errs, fmt.Errorf("queuetest: redelivered Payload = %q, want work (stored copy)", second.Payload))
	}

	if second.Headers["k"] != "v" {
		errs = append(errs, fmt.Errorf("queuetest: redelivered Headers[k] = %q, want v (stored copy)", second.Headers["k"]))
	}

	if second.Attempt <= first.Attempt {
		errs = append(errs, fmt.Errorf("queuetest: redelivered Attempt = %d, want > %d", second.Attempt, first.Attempt))
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	if err := q.Ack(ctx, second); err != nil {
		return fmt.Errorf("queuetest: Ack() error = %w", err)
	}

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		return errf("Pop(after ack) err = %v, want ErrEmpty", err)
	}

	return nil
}

func conformanceNackDrop(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkNackDrop(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkNackDrop proves Nack(drop) discards the message: the topic
// drains to empty.
func checkNackDrop(ctx context.Context, q queue.Queue) error {
	const topic = "nack-drop"

	if err := q.Push(ctx, topic, queue.Payload("work"), nil); err != nil {
		return fmt.Errorf("queuetest: Push() error = %w", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop() error = %w", err)
	}

	if err := q.Nack(ctx, msg, false); err != nil {
		return fmt.Errorf("queuetest: Nack(drop) error = %w", err)
	}

	var errs []error

	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		errs = append(errs, errf("Pop(after drop) err = %v, want ErrEmpty", err))
	}

	if n, err := q.Length(ctx, topic); err != nil || n != 0 {
		errs = append(errs, errf("Length(after drop) = %d,%v want 0,nil", n, err))
	}

	if ok, err := q.IsEmpty(ctx, topic); err != nil || !ok {
		errs = append(errs, errf("IsEmpty(after drop) = %v,%v want true,nil", ok, err))
	}

	return errors.Join(errs...)
}

func conformanceDelayed(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkDelayed(t.Context(), factory(t), DefaultDelayedDelay, DefaultDeliveryTimeout); err != nil {
		t.Fatal(err)
	}
}

// checkDelayed proves non-positive delays deliver immediately while a
// positive delay hides the message until polling observes it. Timeout,
// interval, and delay are parameters so unit tests drive both branches.
func checkDelayed(ctx context.Context, q queue.Queue, delay, timeout time.Duration) error {
	const topic = "delayed"

	// Non-positive delays are immediately available.
	if err := q.PushDelayed(ctx, topic, queue.Payload("now"), nil, 0); err != nil {
		return fmt.Errorf("queuetest: PushDelayed(0) error = %w", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		return fmt.Errorf("queuetest: Pop(immediate) error = %w", err)
	}

	if string(msg.Payload) != "now" {
		return fmt.Errorf("queuetest: Pop(immediate) Payload = %q, want now", msg.Payload)
	}

	if err := q.Ack(ctx, msg); err != nil {
		return fmt.Errorf("queuetest: Ack() error = %w", err)
	}

	if err := q.PushDelayed(ctx, topic, queue.Payload("later"), nil, delay); err != nil {
		return fmt.Errorf("queuetest: PushDelayed() error = %w", err)
	}

	// The delayed message must not be ready yet.
	if _, err := q.Pop(ctx, topic); !errors.Is(err, queue.ErrEmpty) {
		return errf("Pop(before delay) err = %v, want ErrEmpty", err)
	}

	return pollDelivery(ctx, timeout, "delayed message delivered", func(ctx context.Context) error {
		got, err := q.Pop(ctx, topic)
		if err != nil {
			return err
		}

		if string(got.Payload) != "later" {
			return fmt.Errorf("queuetest: Pop(delayed) Payload = %q, want later", got.Payload)
		}

		return nil
	})
}

func conformanceTopicsIsolated(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkTopicsIsolated(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkTopicsIsolated proves pushes to different topics never cross.
func checkTopicsIsolated(ctx context.Context, q queue.Queue) error {
	if err := q.Push(ctx, "topic-a", queue.Payload("a"), nil); err != nil {
		return fmt.Errorf("queuetest: Push(a) error = %w", err)
	}

	if err := q.Push(ctx, "topic-b", queue.Payload("b"), nil); err != nil {
		return fmt.Errorf("queuetest: Push(b) error = %w", err)
	}

	msgB, err := q.Pop(ctx, "topic-b")
	if err != nil {
		return fmt.Errorf("queuetest: Pop(b) error = %w", err)
	}

	var errs []error

	if string(msgB.Payload) != "b" {
		errs = append(errs, fmt.Errorf("queuetest: Pop(b) Payload = %q, want b", msgB.Payload))
	}

	msgA, err := q.Pop(ctx, "topic-a")
	if err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Pop(a) error = %w", err))
		return errors.Join(errs...)
	}

	if string(msgA.Payload) != "a" {
		errs = append(errs, fmt.Errorf("queuetest: Pop(a) Payload = %q, want a", msgA.Payload))
	}

	if err := q.Ack(ctx, msgA); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Ack(a) error = %w", err))
		return errors.Join(errs...)
	}

	if err := q.Ack(ctx, msgB); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Ack(b) error = %w", err))
		return errors.Join(errs...)
	}

	if ok, err := q.IsEmpty(ctx, "topic-a"); err != nil || !ok {
		errs = append(errs, errf("IsEmpty(a) = %v,%v want true,nil", ok, err))
	}

	if ok, err := q.IsEmpty(ctx, "topic-b"); err != nil || !ok {
		errs = append(errs, errf("IsEmpty(b) = %v,%v want true,nil", ok, err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) queue.Queue) {
	t.Helper()

	if err := checkClose(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Name is set, Close is idempotent, and pushes report
// ErrClosed afterwards. Soft mismatches join so all are reported.
func checkClose(ctx context.Context, q queue.Queue) error {
	var errs []error

	if q.Name() == "" {
		errs = append(errs, errors.New("queuetest: Name() is empty, want adapter name"))
	}

	if err := q.Close(); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Close() error = %w", err))
		return errors.Join(errs...)
	}

	if err := q.Close(); err != nil {
		errs = append(errs, fmt.Errorf("queuetest: Close() second error = %w, want nil", err))
	}

	if err := q.Push(ctx, "closed", queue.Payload("x"), nil); !errors.Is(err, queue.ErrClosed) {
		errs = append(errs, errf("Push() err = %v, want ErrClosed", err))
	}

	if err := q.PushDelayed(ctx, "closed", queue.Payload("x"), nil, 0); !errors.Is(err, queue.ErrClosed) {
		errs = append(errs, errf("PushDelayed() err = %v, want ErrClosed", err))
	}

	return errors.Join(errs...)
}

// pollDelivery polls cond until it returns nil or timeout elapses,
// ticking every interval. A payload-mismatch error aborts immediately
// (it is a real assertion, not a poll miss); ErrEmpty keeps polling.
// (It replaces the old eventually(t, ...) helper, whose timeout branch
// was uncoverable: a *testing.T failure cannot be scripted without a
// real test run.)
func pollDelivery(ctx context.Context, timeout time.Duration, msg string, cond func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	var last error

	for {
		if err := cond(ctx); err == nil {
			return nil
		} else if errors.Is(err, queue.ErrEmpty) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Keep polling. Context errors mean a Pop raced the
			// deadline; they are poll misses, not assertion failures.
			last = err
		} else {
			return err
		}

		select {
		case <-ctx.Done():
			if last != nil {
				return fmt.Errorf("condition not met within %v: %s: %w", timeout, msg, last)
			}
			return fmt.Errorf("condition not met within %v: %s", timeout, msg)
		case <-ticker.C:
		}
	}
}
