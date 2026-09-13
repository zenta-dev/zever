package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/queue"
)

// newLiveQueue starts miniredis and opens a queue.
func newLiveQueue(t *testing.T) (queue.Queue, *miniredis.Miniredis) {
	t.Helper()
	_ = zredis.Close()
	s := miniredis.RunT(t)
	q, err := New(queue.Options{Addr: s.Addr(), PollTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q, s
}

func TestRedisLive_PushPopAck(t *testing.T) {
	ctx := context.Background()
	q, _ := newLiveQueue(t)
	topic := "t-pushpopack"
	payload := queue.Payload([]byte("hello"))
	headers := queue.Headers{"h": "v"}

	if err := q.Push(ctx, topic, payload, headers); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if string(msg.Payload) != "hello" {
		t.Errorf("Payload = %q, want hello", string(msg.Payload))
	}
	if got := msg.Headers["h"]; got != "v" {
		t.Errorf("Headers[h] = %q, want v", got)
	}
	if msg.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", msg.Attempt)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	n, err := q.Length(ctx, topic)
	if err != nil {
		t.Fatalf("Length() error = %v", err)
	}
	if n != 0 {
		t.Errorf("Length() = %d, want 0", n)
	}
	empty, err := q.IsEmpty(ctx, topic)
	if err != nil {
		t.Fatalf("IsEmpty() error = %v", err)
	}
	if !empty {
		t.Errorf("IsEmpty() = false, want true")
	}

	// next Pop should be empty (use Background; BLPop truncated to 1s in miniredis, so poll must fire)
	_, err = q.Pop(context.Background(), topic)
	if err == nil {
		t.Fatal("Pop() = nil, want EmptyError")
	}
	if !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("errors.Is(err, ErrEmpty) = false, err = %v", err)
	}
	var emptyErr *queue.EmptyError
	if !errors.As(err, &emptyErr) {
		t.Errorf("errors.As(err, EmptyError) = false, err = %T %v", err, err)
	}
}

func TestRedisLive_PushDelayed(t *testing.T) {
	ctx := context.Background()
	_ = zredis.Close()
	s := miniredis.RunT(t)
	q, err := New(queue.Options{Addr: s.Addr(), PollTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	topic := "t-pushdelayed"
	payload := queue.Payload([]byte("delayed"))
	headers := queue.Headers{"h": "v"}

	if err := q.PushDelayed(ctx, topic, payload, headers, 80*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed() error = %v", err)
	}

	_, err = q.Pop(ctx, topic)
	if err == nil {
		t.Fatal("Pop immediate after PushDelayed = nil, want EmptyError")
	}
	if !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop immediate err = %v, want ErrEmpty", err)
	}

	s.FastForward(100 * time.Millisecond)
	time.Sleep(120 * time.Millisecond)

	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop after delay error = %v", err)
	}
	if string(msg.Payload) != "delayed" {
		t.Errorf("Payload = %q, want delayed", string(msg.Payload))
	}
	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack error = %v", err)
	}

	if err := q.PushDelayed(ctx, topic, queue.Payload([]byte("now")), queue.Headers{"h": "v"}, 0); err != nil {
		t.Fatalf("PushDelayed(0) error = %v", err)
	}
	msg2, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop after PushDelayed(0) error = %v", err)
	}
	if string(msg2.Payload) != "now" {
		t.Errorf("Payload = %q, want now", string(msg2.Payload))
	}
	_ = q.Ack(ctx, msg2)
}

func TestRedisLive_Nack(t *testing.T) {
	ctx := context.Background()
	q, _ := newLiveQueue(t)
	topic := "t-nack"

	if err := q.Push(ctx, topic, queue.Payload([]byte("nack-true")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}
	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop error = %v", err)
	}
	firstID := msg.ID
	if msg.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", msg.Attempt)
	}
	if err := q.Nack(ctx, msg, true); err != nil {
		t.Fatalf("Nack requeue true error = %v", err)
	}
	msg2, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop after Nack requeue error = %v", err)
	}
	if msg2.ID != firstID {
		t.Errorf("ID after Nack = %v, want %v", msg2.ID, firstID)
	}
	if msg2.Attempt != 2 {
		t.Errorf("Attempt after Nack requeue = %d, want 2", msg2.Attempt)
	}
	_ = q.Ack(ctx, msg2)

	if err := q.Push(ctx, topic, queue.Payload([]byte("nack-false")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}
	msg3, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop error = %v", err)
	}
	if err := q.Nack(ctx, msg3, false); err != nil {
		t.Fatalf("Nack requeue false error = %v", err)
	}
	_, err = q.Pop(ctx, topic)
	if !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop after Nack drop err = %v, want ErrEmpty", err)
	}
}

func TestRedisLive_VisibilityReclaim(t *testing.T) {
	ctx := context.Background()
	_ = zredis.Close()
	s := miniredis.RunT(t)
	q, err := New(queue.Options{Addr: s.Addr(), VisibilityTimeout: 120 * time.Millisecond, PollTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	topic := "t-reclaim"
	if err := q.Push(ctx, topic, queue.Payload([]byte("reclaim")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}
	msg, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop error = %v", err)
	}
	firstID := msg.ID
	if msg.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", msg.Attempt)
	}

	s.FastForward(200 * time.Millisecond)
	time.Sleep(220 * time.Millisecond)

	msg2, err := q.Pop(ctx, topic)
	if err != nil {
		t.Fatalf("Pop after reclaim error = %v", err)
	}
	if msg2.ID != firstID {
		t.Errorf("ID after reclaim = %v, want %v", msg2.ID, firstID)
	}
	if msg2.Attempt != 2 {
		t.Errorf("Attempt after reclaim = %d, want 2", msg2.Attempt)
	}
	_ = q.Ack(ctx, msg2)
}

func TestRedisLive_BufferBlocks(t *testing.T) {
	ctx := context.Background()
	_ = zredis.Close()
	s := miniredis.RunT(t)
	q, err := New(queue.Options{Addr: s.Addr(), Buffer: 1, PollTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	topic := "t-buffer"
	if err := q.Push(ctx, topic, queue.Payload([]byte("one")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push1 error = %v", err)
	}

	ctx2, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	err = q.Push(ctx2, topic, queue.Payload([]byte("two")), queue.Headers{"h": "v"})
	if err == nil {
		t.Fatal("Push2 = nil, want buffer timeout error")
	}
	if !strings.Contains(err.Error(), "failed to wait buffer space") && !strings.Contains(err.Error(), "check buffer capacity") {
		t.Errorf("Push2 err = %q, want contains 'failed to wait buffer space' or 'check buffer capacity'", err.Error())
	}
	_, _ = q.Pop(ctx, topic)
}

func TestRedisLive_LengthIsEmpty(t *testing.T) {
	ctx := context.Background()
	q, _ := newLiveQueue(t)
	topic := "t-length"

	n, err := q.Length(ctx, topic)
	if err != nil {
		t.Fatalf("Length empty error = %v", err)
	}
	if n != 0 {
		t.Errorf("Length empty = %d, want 0", n)
	}
	empty, err := q.IsEmpty(ctx, topic)
	if err != nil {
		t.Fatalf("IsEmpty empty error = %v", err)
	}
	if !empty {
		t.Errorf("IsEmpty empty = false, want true")
	}

	if err := q.Push(ctx, topic, queue.Payload([]byte("a")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}
	if err := q.Push(ctx, topic, queue.Payload([]byte("b")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}

	n, err = q.Length(ctx, topic)
	if err != nil {
		t.Fatalf("Length after push error = %v", err)
	}
	if n != 2 {
		t.Errorf("Length after push = %d, want 2", n)
	}
	empty, err = q.IsEmpty(ctx, topic)
	if err != nil {
		t.Fatalf("IsEmpty after push error = %v", err)
	}
	if empty {
		t.Errorf("IsEmpty after push = true, want false")
	}
}

func TestRedisLive_PingFailure_redactsPassword(t *testing.T) {
	_ = zredis.Close()
	s := miniredis.RunT(t)
	dead := s.Addr()
	s.Close()

	_, err := New(queue.Options{Addr: "redis://:s3cret@" + dead})
	if err == nil {
		t.Fatal("New() = nil, want ping error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks password: %v", err)
	}
	if !strings.Contains(err.Error(), "xxxxx") {
		t.Errorf("error missing redacted password: %v", err)
	}
	_ = zredis.Close()
}

func TestRedisLive_BufferDelayedBlocks(t *testing.T) {
	ctx := context.Background()
	_ = zredis.Close()
	s := miniredis.RunT(t)
	q, err := New(queue.Options{Addr: s.Addr(), Buffer: 1, PollTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	topic := "t-buffer-delayed"
	if err := q.PushDelayed(ctx, topic, queue.Payload([]byte("one")), queue.Headers{"h": "v"}, 80*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed1 error = %v", err)
	}
	ctx2, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	err = q.PushDelayed(ctx2, topic, queue.Payload([]byte("two")), queue.Headers{"h": "v"}, 80*time.Millisecond)
	if err == nil {
		t.Fatal("PushDelayed2 = nil, want buffer timeout")
	}
	if !strings.Contains(err.Error(), "failed to wait buffer space") && !strings.Contains(err.Error(), "check buffer capacity") {
		t.Errorf("PushDelayed2 err = %q, want buffer error", err.Error())
	}
}

func TestRedisLive_BlockingPop_claimsConcurrentPush(t *testing.T) {
	ctx := context.Background()
	q, _ := newLiveQueue(t)
	topic := "t-blocking"

	type popRes struct {
		msg queue.Message
		err error
	}
	ch := make(chan popRes, 1)
	go func() {
		m, err := q.Pop(ctx, topic)
		ch <- popRes{msg: m, err: err}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := q.Push(ctx, topic, queue.Payload([]byte("concurrent")), queue.Headers{"h": "v"}); err != nil {
		t.Fatalf("Push concurrent error = %v", err)
	}

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("blocking Pop error = %v", res.err)
		}
		if string(res.msg.Payload) != "concurrent" {
			t.Errorf("Payload = %q, want concurrent", string(res.msg.Payload))
		}
		if res.msg.Attempt != 1 {
			t.Errorf("Attempt = %d, want 1", res.msg.Attempt)
		}
		_ = q.Ack(ctx, res.msg)
	case <-time.After(2 * time.Second):
		t.Fatal("blocking Pop timed out")
	}
}

func TestRedisLive_ServerDown_opsWrap(t *testing.T) {
	ctx := context.Background()
	q, s := newLiveQueue(t)
	topic := "t-serverdown"
	s.Close()

	if err := q.Push(ctx, topic, queue.Payload([]byte("p")), queue.Headers{"h": "v"}); err == nil || errors.Is(err, queue.ErrClosed) || errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Push after close err = %v, want wrapped transport error not ErrClosed/ErrEmpty", err)
	}
	if err := q.PushDelayed(ctx, topic, queue.Payload([]byte("p")), queue.Headers{"h": "v"}, 0); err == nil || errors.Is(err, queue.ErrClosed) || errors.Is(err, queue.ErrEmpty) {
		t.Errorf("PushDelayed after close err = %v, want wrapped transport error", err)
	}
	if _, err := q.Pop(ctx, topic); err == nil || errors.Is(err, queue.ErrClosed) || errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Pop after close err = %v, want wrapped transport error", err)
	}
	if _, err := q.Length(ctx, topic); err == nil || errors.Is(err, queue.ErrClosed) || errors.Is(err, queue.ErrEmpty) {
		t.Errorf("Length after close err = %v, want wrapped transport error", err)
	}
}
