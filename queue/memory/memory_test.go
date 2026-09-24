package memory

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/queue"
)

func newQueue(t *testing.T, opts queue.Options) queue.Queue {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	const timeout = 3 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("eventually timeout %s: %s", timeout, msg)
	}
}

// TestMemory_PushPop_Ack_roundTrip verifies push/pop/ack cycle, deep-copy and EmptyError.
func TestMemory_PushPop_Ack_roundTrip(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	topic := "jobs"
	payload := queue.Payload([]byte("hi"))
	headers := queue.Headers{"a": "b"}

	if err := q.Push(context.Background(), topic, payload, headers); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// NOTE: Push does not deep-copy; Pop clone provides isolation.
	origPayload := string(payload)
	origHeaderA := headers["a"]

	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if msg.Attempt != 1 {
		t.Fatalf("Attempt = %d want 1", msg.Attempt)
	}
	if string(msg.Payload) != origPayload {
		t.Fatalf("Payload = %q want %q", string(msg.Payload), origPayload)
	}
	if msg.Headers["a"] != origHeaderA {
		t.Fatalf("Headers[a] = %q want %q", msg.Headers["a"], origHeaderA)
	}
	msg.Payload[0] = 'y'
	msg.Headers["a"] = "changed"

	n, err := q.Length(context.Background(), topic)
	if err != nil {
		t.Fatalf("Length: %v", err)
	}
	if n != 0 {
		t.Fatalf("Length = %d want 0", n)
	}
	empty, err := q.IsEmpty(context.Background(), topic)
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if !empty {
		t.Fatalf("IsEmpty = false want true (inflight not counted)")
	}

	if ackErr := q.Ack(context.Background(), msg); ackErr != nil {
		t.Fatalf("Ack: %v", ackErr)
	}

	n, _ = q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length after Ack = %d want 0", n)
	}
	empty, _ = q.IsEmpty(context.Background(), topic)
	if !empty {
		t.Fatalf("IsEmpty after Ack = false want true")
	}

	_, err = q.Pop(context.Background(), topic)
	if err == nil {
		t.Fatalf("Pop expected empty error")
	}
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop err = %v want ErrEmpty", err)
	}
	var ee *queue.EmptyError
	if errors.As(err, &ee) {
		if ee.Topic != topic {
			t.Fatalf("EmptyError Topic = %q want %q", ee.Topic, topic)
		}
	}

	payload2 := queue.Payload([]byte("hi2"))
	if pushErr := q.Push(context.Background(), topic, payload2, nil); pushErr != nil {
		t.Fatalf("Push2: %v", pushErr)
	}
	msg2, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop2: %v", err)
	}
	if string(msg2.Payload) != "hi2" {
		t.Fatalf("Payload2 = %q want hi2", string(msg2.Payload))
	}
	_ = q.Ack(context.Background(), msg2)
}

func TestMemory_PushDelayed(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	topic := "delayedTopic"

	if err := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("immediate")), nil, 0); err != nil {
		t.Fatalf("PushDelayed 0: %v", err)
	}
	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop immediate (delay 0): %v", err)
	}
	if string(msg.Payload) != "immediate" {
		t.Fatalf("payload immediate = %q", string(msg.Payload))
	}
	_ = q.Ack(context.Background(), msg)

	if pushDelayErr := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("immediate2")), nil, -5*time.Millisecond); pushDelayErr != nil {
		t.Fatalf("PushDelayed -5ms: %v", pushDelayErr)
	}
	msg, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop immediate (delay -5ms): %v", err)
	}
	if string(msg.Payload) != "immediate2" {
		t.Fatalf("payload immediate2 = %q", string(msg.Payload))
	}
	_ = q.Ack(context.Background(), msg)

	if pushLaterErr := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("later")), nil, 40*time.Millisecond); pushLaterErr != nil {
		t.Fatalf("PushDelayed 40ms: %v", pushLaterErr)
	}
	if pushNowErr := q.Push(context.Background(), topic, queue.Payload([]byte("now")), nil); pushNowErr != nil {
		t.Fatalf("Push now: %v", err)
	}
	msg, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop now: %v", err)
	}
	if string(msg.Payload) != "now" {
		t.Fatalf("Pop now payload = %q want now", string(msg.Payload))
	}
	_ = q.Ack(context.Background(), msg)

	_, err = q.Pop(context.Background(), topic)
	if err == nil {
		t.Fatalf("Pop delayed should be empty before ready")
	}
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop delayed err = %v want ErrEmpty", err)
	}

	eventually(t, func() bool {
		n, _ := q.Length(context.Background(), topic)
		return n == 1
	}, "delayed entry not promoted")

	msg, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after delay: %v", err)
	}
	if string(msg.Payload) != "later" {
		t.Fatalf("delayed payload = %q want later", string(msg.Payload))
	}
	_ = q.Ack(context.Background(), msg)

	topic2 := "delayedSort"
	if err := q.PushDelayed(context.Background(), topic2, queue.Payload([]byte("d40")), nil, 40*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed d40: %v", err)
	}
	if err := q.PushDelayed(context.Background(), topic2, queue.Payload([]byte("d20")), nil, 20*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed d20: %v", err)
	}
	if err := q.PushDelayed(context.Background(), topic2, queue.Payload([]byte("d60")), nil, 60*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed d60: %v", err)
	}
	eventually(t, func() bool {
		n, _ := q.Length(context.Background(), topic2)
		return n == 3
	}, "all delayed entries not promoted")

	expected := []string{"d20", "d40", "d60"}
	for i, want := range expected {
		m, err := q.Pop(context.Background(), topic2)
		if err != nil {
			t.Fatalf("Pop sorted %d: %v", i, err)
		}
		if string(m.Payload) != want {
			t.Fatalf("Pop sorted %d = %q want %q", i, string(m.Payload), want)
		}
		_ = q.Ack(context.Background(), m)
	}
}

func TestMemory_Nack(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	topic := "nackTopic"

	if err := q.Push(context.Background(), topic, queue.Payload([]byte("msg1")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if msg.Attempt != 1 {
		t.Fatalf("Attempt = %d want 1", msg.Attempt)
	}
	if nackErr := q.Nack(context.Background(), msg, true); nackErr != nil {
		t.Fatalf("Nack requeue true: %v", nackErr)
	}
	n, _ := q.Length(context.Background(), topic)
	if n != 1 {
		t.Fatalf("Length after Nack requeue true = %d want 1", n)
	}
	msg2, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after Nack requeue: %v", err)
	}
	if msg2.ID != msg.ID {
		t.Fatalf("ID mismatch after requeue")
	}
	if msg2.Attempt != 2 {
		t.Fatalf("Attempt after requeue = %d want 2", msg2.Attempt)
	}

	if nackStaleErr := q.Nack(context.Background(), msg, true); nackStaleErr != nil {
		t.Fatalf("Nack stale: %v", nackStaleErr)
	}
	n, _ = q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length after stale Nack = %d want 0", n)
	}
	if nackErr2 := q.Nack(context.Background(), msg2, false); nackErr2 != nil {
		t.Fatalf("Nack requeue false: %v", nackErr2)
	}
	n, _ = q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length after Nack requeue false = %d want 0", n)
	}
	empty, _ := q.IsEmpty(context.Background(), topic)
	if !empty {
		t.Fatalf("IsEmpty after drop false")
	}
	_, err = q.Pop(context.Background(), topic)
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop after drop err = %v want ErrEmpty", err)
	}

	if pushMsg2Err := q.Push(context.Background(), topic, queue.Payload([]byte("msg2")), nil); pushMsg2Err != nil {
		t.Fatalf("Push msg2: %v", pushMsg2Err)
	}
	m, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop msg2: %v", err)
	}
	if nackMErr := q.Nack(context.Background(), m, false); nackMErr != nil {
		t.Fatalf("Nack false: %v", err)
	}
	n, _ = q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length after Nack false = %d want 0", n)
	}

	if pushMsg3Err := q.Push(context.Background(), topic, queue.Payload([]byte("msg3")), nil); pushMsg3Err != nil {
		t.Fatalf("Push msg3: %v", pushMsg3Err)
	}
	m, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop msg3: %v", err)
	}
	stale := m
	if nackMsg3Err := q.Nack(context.Background(), m, true); nackMsg3Err != nil {
		t.Fatalf("Nack requeue msg3: %v", err)
	}
	if ackStaleErr := q.Ack(context.Background(), stale); ackStaleErr != nil {
		t.Fatalf("Ack stale: %v", ackStaleErr)
	}
	n, _ = q.Length(context.Background(), topic)
	if n != 1 {
		t.Fatalf("Length after stale Ack = %d want 1 (requeued still there)", n)
	}
	m2, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after stale Ack: %v", err)
	}
	if m2.Attempt != 2 {
		t.Fatalf("Attempt after stale Ack = %d want 2", m2.Attempt)
	}
	_ = q.Ack(context.Background(), m2)
}

func TestMemory_VisibilityTimeout_reclaim(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: 40 * time.Millisecond})
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}

	topic := "visTopic"
	if err := q.Push(context.Background(), topic, queue.Payload([]byte("vis")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	id := msg.ID

	n, _ := q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length inflight = %d want 0", n)
	}

	// Poll for the reclaim: Pop reclaims expired visibility synchronously,
	// so retry until the 40ms timeout lapses instead of sleeping past it.
	var msg2 queue.Message
	eventually(t, func() bool {
		m, popErr := q.Pop(context.Background(), topic)
		if popErr != nil {
			return false
		}
		if m.ID != id || m.Attempt != 2 {
			_ = q.Ack(context.Background(), m)
			return false
		}
		msg2 = m
		return true
	}, "visibility reclaim")
	_ = q.Ack(context.Background(), msg2)

	if pushVis2Err := q.Push(context.Background(), topic, queue.Payload([]byte("vis2")), nil); pushVis2Err != nil {
		t.Fatalf("Push vis2: %v", pushVis2Err)
	}
	_, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop vis2: %v", err)
	}
	tq := ma.getTopic(topic)
	if tq == nil {
		t.Fatalf("topic missing")
	}
	tq.mu.Lock()
	tq.visHeap = nil
	tq.mu.Unlock()

	// No sleep: poll until the 40ms visibility timeout lapses; each Pop
	// attempt reclaims expired entries synchronously.
	eventually(t, func() bool {
		innerN, _ := q.Length(context.Background(), topic)
		if innerN == 1 {
			return true
		}
		m2, popErr := q.Pop(context.Background(), topic)
		if popErr == nil {
			if m2.Attempt == 2 {
				_ = q.Ack(context.Background(), m2)
				return true
			}
			_ = q.Ack(context.Background(), m2)
			return true
		}
		return false
	}, "reclaimFromMap not triggered")

	n2, _ := q.Length(context.Background(), topic)
	if n2 == 1 {
		m, popErr2 := q.Pop(context.Background(), topic)
		if popErr2 != nil {
			t.Fatalf("Pop after reclaimFromMap: %v", popErr2)
		}
		if m.Attempt != 2 {
			t.Fatalf("reclaimFromMap Attempt = %d want 2", m.Attempt)
		}
		_ = q.Ack(context.Background(), m)
	}

	if pushVis3Err := q.Push(context.Background(), topic, queue.Payload([]byte("vis3")), nil); pushVis3Err != nil {
		t.Fatalf("Push vis3: %v", err)
	}
	msg, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop vis3: %v", err)
	}
	tq = ma.getTopic(topic)
	tq.mu.Lock()
	if inf, ok := tq.inflight[msg.ID]; ok {
		staleDeadline := inf.deadline
		inf.deadline = staleDeadline.Add(time.Hour)
		tq.inflight[msg.ID] = inf
		_ = staleDeadline
	}
	tq.mu.Unlock()
	// No sleep: a future deadline keeps Length at 0 synchronously.
	n, _ = q.Length(context.Background(), topic)
	if n != 0 {
		t.Fatalf("Length should still be 0 for future deadline, got %d", n)
	}
	tq.mu.Lock()
	if inf, ok := tq.inflight[msg.ID]; ok {
		inf.deadline = time.Now().Add(-10 * time.Millisecond)
		tq.inflight[msg.ID] = inf
	}
	// Backdate the heap entry too (keeping the deadline mismatch the test
	// covers): the heap top must already be expired for the reclaim to run
	// synchronously inside Pop instead of after a wall-clock sleep.
	for _, e := range tq.visHeap {
		if e.id == msg.ID {
			e.deadline = time.Now().Add(-5 * time.Millisecond)
		}
	}
	tq.mu.Unlock()
	// No sleep: Pop reclaims the past-due deadline synchronously.
	msg2, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after mismatch reclaim: %v", err)
	}
	if msg2.Attempt != 2 {
		t.Fatalf("mismatch reclaim Attempt = %d want 2", msg2.Attempt)
	}
	_ = q.Ack(context.Background(), msg2)
}

func TestMemory_Buffer_backpressure(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 2, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	topic := "bufferTopic"

	if err := q.Push(context.Background(), topic, queue.Payload([]byte("1")), nil); err != nil {
		t.Fatalf("Push1: %v", err)
	}
	if err := q.Push(context.Background(), topic, queue.Payload([]byte("2")), nil); err != nil {
		t.Fatalf("Push2: %v", err)
	}
	n, _ := q.Length(context.Background(), topic)
	if n != 2 {
		t.Fatalf("Length = %d want 2", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	err := q.Push(ctx, topic, queue.Payload([]byte("3")), nil)
	if err == nil {
		t.Fatalf("Push expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Push err = %v want wrap DeadlineExceeded/Canceled", err)
	}
	if !strings.Contains(err.Error(), "queue: failed to wait for queue space") {
		t.Fatalf("Push err message = %q missing queue: failed to wait", err.Error())
	}

	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	_ = q.Ack(context.Background(), msg)
	if pushAfterFreeErr := q.Push(context.Background(), topic, queue.Payload([]byte("3")), nil); pushAfterFreeErr != nil {
		t.Fatalf("Push after free: %v", pushAfterFreeErr)
	}
	for {
		m2, popErr3 := q.Pop(context.Background(), topic)
		if popErr3 != nil {
			break
		}
		_ = q.Ack(context.Background(), m2)
	}

	if pushAErr := q.Push(context.Background(), topic, queue.Payload([]byte("a")), nil); pushAErr != nil {
		t.Fatalf("Push a: %v", pushAErr)
	}
	if pushBErr := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("b")), nil, 200*time.Millisecond); pushBErr != nil {
		t.Fatalf("PushDelayed b: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel2()
	err = q.PushDelayed(ctx2, topic, queue.Payload([]byte("c")), nil, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("PushDelayed expected timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("PushDelayed err = %v want DeadlineExceeded/Canceled", err)
	}

	topic3 := "bufferTopic2"
	if pushXErr := q.Push(context.Background(), topic3, queue.Payload([]byte("x")), nil); pushXErr != nil {
		t.Fatalf("Push x: %v", pushXErr)
	}
	if pushYErr := q.Push(context.Background(), topic3, queue.Payload([]byte("y")), nil); pushYErr != nil {
		t.Fatalf("Push y: %v", err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel3()
	err = q.PushDelayed(ctx3, topic3, queue.Payload([]byte("z")), nil, 0)
	if err == nil {
		t.Fatalf("PushDelayed 0 expected timeout when ready full")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("PushDelayed 0 err = %v", err)
	}
	m, _ := q.Pop(context.Background(), topic3)
	_ = q.Ack(context.Background(), m)
}

func TestMemory_LengthAndIsEmpty(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond})

	unknown := "unknownTopicXYZ"
	n, err := q.Length(context.Background(), unknown)
	if err != nil {
		t.Fatalf("Length unknown: %v", err)
	}
	if n != 0 {
		t.Fatalf("Length unknown = %d want 0", n)
	}
	empty, err := q.IsEmpty(context.Background(), unknown)
	if err != nil {
		t.Fatalf("IsEmpty unknown: %v", err)
	}
	if !empty {
		t.Fatalf("IsEmpty unknown false")
	}

	if err := q.Push(context.Background(), unknown, queue.Payload([]byte("hi")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	n, _ = q.Length(context.Background(), unknown)
	if n != 1 {
		t.Fatalf("Length after Push = %d want 1", n)
	}
	empty, _ = q.IsEmpty(context.Background(), unknown)
	if empty {
		t.Fatalf("IsEmpty after Push true want false")
	}
}

func TestMemory_PopEmpty_and_Cancelled(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	_, err := q.Pop(context.Background(), "noSuchTopic")
	if err == nil {
		t.Fatalf("Pop empty expected error")
	}
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop empty err = %v want ErrEmpty", err)
	}
	topic := "emptyExisting"
	if pushOneErr := q.Push(context.Background(), topic, queue.Payload([]byte("one")), nil); pushOneErr != nil {
		t.Fatalf("Push: %v", pushOneErr)
	}
	m, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	_ = q.Ack(context.Background(), m)
	topic2 := "emptyPoll"
	if pushTmpErr := q.Push(context.Background(), topic2, queue.Payload([]byte("tmp")), nil); pushTmpErr != nil {
		t.Fatalf("Push tmp: %v", err)
	}
	m, err = q.Pop(context.Background(), topic2)
	if err != nil {
		t.Fatalf("Pop tmp: %v", err)
	}
	_ = q.Ack(context.Background(), m)
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}
	tq, _ := ma.topic(topic2)
	_ = tq
	_, err = q.Pop(context.Background(), topic2)
	if err == nil {
		t.Fatalf("Pop emptyExisting expected error")
	}
	var ee *queue.EmptyError
	if errors.As(err, &ee) {
		if ee.Topic != topic2 {
			t.Fatalf("EmptyError Topic = %q want %q", ee.Topic, topic2)
		}
		if !errors.Is(err, queue.ErrEmpty) {
			t.Fatalf("EmptyError not Is ErrEmpty")
		}
	} else if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("Pop empty err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = q.Pop(ctx, "cancelTopic")
	if err == nil {
		t.Fatalf("Pop cancelled expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Pop cancelled err = %v want Canceled", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "queue: pop cancelled") {
		t.Fatalf("Pop cancelled err message = %q missing queue: Pop cancelled", err.Error())
	}

	topic3 := "cancelExisting"
	tq3, _ := ma.topic(topic3)
	_ = tq3
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	_, err = q.Pop(ctx2, topic3)
	if err == nil {
		t.Fatalf("Pop cancelled existing expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Pop cancelled existing err = %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "queue: pop cancelled") {
		t.Fatalf("Pop cancelled existing message = %q", err.Error())
	}

	ctx3, cancel3 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel3()
	<-ctx3.Done() // wait for expiry via channel, not sleep
	_, err = q.Pop(ctx3, topic3)
	if err == nil {
		t.Fatalf("Pop deadline expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Pop deadline err = %v", err)
	}
}

func TestMemory_Close(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond})
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}

	if err := q.Push(context.Background(), "closeTopic", queue.Payload([]byte("hi")), nil); err != nil {
		t.Fatalf("Push before Close: %v", err)
	}

	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	err := q.Push(context.Background(), "closeTopic", queue.Payload([]byte("hi2")), nil)
	if !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("Push after Close err = %v want ErrClosed", err)
	}
	err = q.PushDelayed(context.Background(), "closeTopic", queue.Payload([]byte("hi2")), nil, 10*time.Millisecond)
	if !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("PushDelayed after Close err = %v want ErrClosed", err)
	}
	_, err = ma.topic("any")
	if !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("topic after Close err = %v want ErrClosed", err)
	}
	msg := queue.NewMessage("closedTopic", queue.Payload([]byte("x")), nil)
	if err := q.Ack(context.Background(), msg); !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("Ack after Close on missing topic err = %v want ErrClosed", err)
	}
	if err := q.Nack(context.Background(), msg, true); !errors.Is(err, queue.ErrClosed) {
		t.Fatalf("Nack after Close on missing topic err = %v want ErrClosed", err)
	}

	if q.Name() != "memory" {
		t.Fatalf("Name = %q want memory", q.Name())
	}
}

func TestMemory_CloneIsolation(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond})

	topic := "cloneTopic"
	payload := queue.Payload([]byte("original"))
	headers := queue.Headers{"k": "v", "a": "b"}

	if err := q.Push(context.Background(), topic, payload, headers); err != nil {
		t.Fatalf("Push: %v", err)
	}

	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if string(msg.Payload) != "original" {
		t.Fatalf("Clone payload = %q want original", string(msg.Payload))
	}
	if msg.Headers["k"] != "v" {
		t.Fatalf("Clone headers k = %q want v", msg.Headers["k"])
	}
	if _, ok := msg.Headers["new"]; ok {
		t.Fatalf("Clone headers leaked new")
	}
	if msg.Headers["a"] != "b" {
		t.Fatalf("Clone headers a = %q want b", msg.Headers["a"])
	}

	origID := msg.ID
	msg.Payload[0] = 'Y'
	msg.Headers["k"] = "mutated"
	if nackErr := q.Nack(context.Background(), msg, true); nackErr != nil {
		t.Fatalf("Nack: %v", nackErr)
	}
	msg2, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after Nack: %v", err)
	}
	if msg2.ID != origID {
		t.Fatalf("ID after Nack mismatch")
	}
	if string(msg2.Payload) != "original" {
		t.Fatalf("After Nack payload = %q want original", string(msg2.Payload))
	}
	if msg2.Headers["k"] != "v" {
		t.Fatalf("After Nack headers = %q want v", msg2.Headers["k"])
	}
	msg2.Payload[0] = 'Z'
	_ = q.Ack(context.Background(), msg2)

	if pushNilErr := q.Push(context.Background(), topic, nil, nil); pushNilErr != nil {
		t.Fatalf("Push nil: %v", pushNilErr)
	}
	m3, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop nil: %v", err)
	}
	if len(m3.Payload) != 0 {
		t.Fatalf("Payload nil clone = %v", m3.Payload)
	}
	_ = q.Ack(context.Background(), m3)

	orig := queue.NewMessage(topic, queue.Payload([]byte("abc")), queue.Headers{"h": "1"})
	cloned := orig.Clone()
	cloned.Payload[0] = 'z'
	cloned.Headers["h"] = "2"
	if string(orig.Payload) != "abc" || orig.Headers["h"] != "1" {
		t.Fatalf("Message.Clone not deep")
	}
}

func TestMemory_ConcurrentMixed(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10000, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

	const (
		goroutines = 8
		ops        = 100
	)
	shared := "sharedTopic"

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*ops*2)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				topic := fmt.Sprintf("distinct-%d", id)
				if err := q.Push(context.Background(), topic, queue.Payload([]byte(fmt.Sprintf("d-%d-%d", id, j))), nil); err != nil {
					errCh <- fmt.Errorf("push distinct: %w", err)
					return
				}
				if err := q.Push(context.Background(), shared, queue.Payload([]byte(fmt.Sprintf("s-%d-%d", id, j))), nil); err != nil {
					errCh <- fmt.Errorf("push shared: %w", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < goroutines; i++ {
		topic := fmt.Sprintf("distinct-%d", i)
		n, _ := q.Length(context.Background(), topic)
		if n != ops {
			t.Fatalf("Length %s = %d want %d", topic, n, ops)
		}
	}
	n, _ := q.Length(context.Background(), shared)
	if n != goroutines*ops {
		t.Fatalf("Length shared = %d want %d", n, goroutines*ops)
	}

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				msg, err := q.Pop(context.Background(), shared)
				if err != nil {
					if errors.Is(err, queue.ErrEmpty) {
						return
					}
					errCh <- fmt.Errorf("pop shared: %w", err)
					return
				}
				if err := q.Ack(context.Background(), msg); err != nil {
					errCh <- fmt.Errorf("ack: %w", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			topic := fmt.Sprintf("distinct-%d", id)
			for {
				msg, err := q.Pop(context.Background(), topic)
				if err != nil {
					if errors.Is(err, queue.ErrEmpty) {
						return
					}
					errCh <- fmt.Errorf("pop distinct %d: %w", id, err)
					return
				}
				if msg.Attempt%2 == 0 {
					_ = q.Ack(context.Background(), msg)
				} else {
					_ = q.Nack(context.Background(), msg, true)
					m2, err := q.Pop(context.Background(), topic)
					if err != nil {
						errCh <- fmt.Errorf("pop after nack %d: %w", id, err)
						return
					}
					_ = q.Ack(context.Background(), m2)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent error: %v", err)
	}

	n, _ = q.Length(context.Background(), shared)
	if n != 0 {
		t.Fatalf("shared not empty after concurrent: %d", n)
	}
	for i := 0; i < goroutines; i++ {
		topic := fmt.Sprintf("distinct-%d", i)
		n, _ := q.Length(context.Background(), topic)
		if n != 0 {
			t.Fatalf("distinct %s not empty: %d", topic, n)
		}
	}
}

func TestMemoryPromoter_lifecycle(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}

	topic := "promoterTopic"

	if err := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("first")), nil, 30*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed first: %v", err)
	}
	tq := ma.getTopic(topic)
	tq.mu.Lock()
	on := tq.promoterOn
	done := tq.done
	tq.mu.Unlock()
	if !on {
		t.Fatalf("promoterOn false after first delayed")
	}
	tq.mu.Lock()
	tq.startPromoterLocked()
	tq.mu.Unlock()
	tq.mu.Lock()
	done2 := tq.done
	on2 := tq.promoterOn
	tq.mu.Unlock()
	if !on2 {
		t.Fatalf("promoterOn false after second start")
	}
	if fmt.Sprintf("%p", done) != fmt.Sprintf("%p", done2) {
		t.Fatalf("promoter done channel changed on idempotent call")
	}

	eventually(t, func() bool {
		n, _ := q.Length(context.Background(), topic)
		return n == 1
	}, "promoter not promoted first")

	eventually(t, func() bool {
		tq.mu.Lock()
		on := tq.promoterOn
		rem := len(tq.delayed)
		tq.mu.Unlock()
		return !on && rem == 0
	}, "promoter not turned off after promotion")

	if err := q.PushDelayed(context.Background(), topic, queue.Payload([]byte("second")), nil, 20*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed second: %v", err)
	}
	eventually(t, func() bool {
		n, _ := q.Length(context.Background(), topic)
		return n == 2
	}, "promoter not restarted")

	m, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after promoter restart: %v", err)
	}
	_ = q.Ack(context.Background(), m)
	m, err = q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop second: %v", err)
	}
	_ = q.Ack(context.Background(), m)

	eventually(t, func() bool {
		tq2 := ma.getTopic(topic)
		if tq2 == nil {
			return true
		}
		tq2.mu.Lock()
		rem := len(tq2.delayed)
		on := tq2.promoterOn
		tq2.mu.Unlock()
		return rem == 0 && !on
	}, "promoter not off after drain")

	topic2 := "promoterWake"
	if pushLater60Err := q.PushDelayed(context.Background(), topic2, queue.Payload([]byte("later60")), nil, 60*time.Millisecond); pushLater60Err != nil {
		t.Fatalf("PushDelayed later60: %v", pushLater60Err)
	}
	// Yield so the promoter goroutine can park in waitForPromote and take
	// the wake path below. Either way the order assertion holds.
	runtime.Gosched()
	if pushEarlier20Err := q.PushDelayed(context.Background(), topic2, queue.Payload([]byte("earlier20")), nil, 20*time.Millisecond); pushEarlier20Err != nil {
		t.Fatalf("PushDelayed earlier20: %v", err)
	}
	eventually(t, func() bool {
		n, _ := q.Length(context.Background(), topic2)
		return n == 2
	}, "promoter wake not promoted both")

	m1, err := q.Pop(context.Background(), topic2)
	if err != nil {
		t.Fatalf("Pop wake 1: %v", err)
	}
	m2, err := q.Pop(context.Background(), topic2)
	if err != nil {
		t.Fatalf("Pop wake 2: %v", err)
	}
	if string(m1.Payload) != "earlier20" || string(m2.Payload) != "later60" {
		t.Fatalf("promoter wake order = %q,%q want earlier20,later60", string(m1.Payload), string(m2.Payload))
	}
	_ = q.Ack(context.Background(), m1)
	_ = q.Ack(context.Background(), m2)
}

// Additional coverage: visibilityHeap and helpers.
func TestMemory_VisibilityHeap(t *testing.T) {
	t.Parallel()
	testHeap(t)
}

func testHeap(t *testing.T) {
	t.Helper()
	h := visibilityHeap{}
	h.Push(&visibilityEntry{deadline: time.Now().Add(10 * time.Millisecond)})
	h.Push(&visibilityEntry{deadline: time.Now().Add(5 * time.Millisecond)})
	if h.Len() != 2 {
		t.Fatalf("heap Len %d want 2", h.Len())
	}
	if !h.Less(1, 0) {
		t.Fatalf("Less(1, 0) = false, want true")
	}
	h.Swap(0, 1)
	if h[0].deadline.After(h[1].deadline) {
		t.Fatalf("Swap failed")
	}
	v := h.Pop()
	if v == nil {
		t.Fatalf("Pop nil")
	}
}

// Test options defaults.
func TestMemory_NewDefaults(t *testing.T) {
	t.Parallel()
	q, err := New(queue.Options{})
	if err != nil {
		t.Fatalf("New defaults: %v", err)
	}
	defer q.Close()
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}
	if ma.visibilityTimeout != 30*time.Second {
		t.Fatalf("visibilityTimeout default %v", ma.visibilityTimeout)
	}
	if ma.pollTimeout != 5*time.Second {
		t.Fatalf("pollTimeout default %v", ma.pollTimeout)
	}
	if ma.buffer != 10000 {
		t.Fatalf("buffer default %d", ma.buffer)
	}
}

func TestMemory_SignalSpace(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 2, PollTimeout: 20 * time.Millisecond})
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}
	tq, _ := ma.topic("sigSpace")
	done := make(chan struct{})
	go func() {
		<-tq.spaceCh
		close(done)
	}()
	// Retry the non-blocking signal until the waiter receives it instead of
	// sleeping to let the goroutine park on spaceCh.
	eventually(t, func() bool {
		tq.signalSpace()
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, "signalSpace not received by waiter")
}

func TestMemory_ReclaimExpired_direct(t *testing.T) {
	t.Parallel()
	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: 40 * time.Millisecond})
	ma, ok := q.(*memoryAdapter)
	if !ok {
		t.Fatalf("type assert memoryAdapter failed")
	}
	_ = ma
	topic := "reclaimDirect"
	if err := q.Push(context.Background(), topic, queue.Payload([]byte("a")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	msg, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	tq := ma.getTopic(topic)
	tq.mu.Lock()
	if inf, ok := tq.inflight[msg.ID]; ok {
		inf.deadline = time.Now().Add(-time.Second)
		tq.inflight[msg.ID] = inf
		if tq.visHeap.Len() > 0 {
			tq.visHeap[0].deadline = time.Now().Add(-time.Second)
		}
	}
	tq.mu.Unlock()
	tq.reclaimExpired(40 * time.Millisecond)
	n, _ := q.Length(context.Background(), topic)
	if n != 1 {
		t.Fatalf("reclaimExpired Length %d want 1", n)
	}
	m2, err := q.Pop(context.Background(), topic)
	if err != nil {
		t.Fatalf("Pop after reclaimExpired: %v", err)
	}
	if m2.Attempt != 2 {
		t.Fatalf("Attempt %d want 2", m2.Attempt)
	}
	_ = q.Ack(context.Background(), m2)
}
