package schedulertest

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

// TestEdgeStubQueueContract exercises the kit stub queue's otherwise unused
// Queue methods so coverage reflects the stub's full contract.
func TestEdgeStubQueueContract(t *testing.T) {
	t.Parallel()

	s := newStubQueue()
	ctx := t.Context()

	if err := s.Push(ctx, "jobs", queue.Payload("hi"), queue.Headers{"k": "v"}); err != nil {
		t.Fatalf("Push error = %v", err)
	}

	if err := s.PushDelayed(ctx, "jobs", queue.Payload("later"), nil, time.Second); err != nil {
		t.Fatalf("PushDelayed error = %v", err)
	}

	if n, err := s.Length(ctx, "jobs"); err != nil || n != 1 {
		t.Fatalf("Length = %d,%v want 1,nil", n, err)
	}

	if ok, err := s.IsEmpty(ctx, "jobs"); err != nil || ok {
		t.Fatalf("IsEmpty = %v,%v want false,nil", ok, err)
	}

	msg, err := s.Pop(ctx, "jobs")
	if err != nil {
		t.Fatalf("Pop error = %v", err)
	}

	if string(msg.Payload) != "hi" {
		t.Fatalf("Pop Payload = %q, want hi", msg.Payload)
	}

	if err := s.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack error = %v", err)
	}

	if err := s.Nack(ctx, msg, true); err != nil {
		t.Fatalf("Nack error = %v", err)
	}

	if _, err := s.Pop(ctx, "jobs"); err == nil {
		t.Fatal("Pop(drained) = nil, want ErrEmpty")
	}

	if n, err := s.Length(ctx, "jobs"); err != nil || n != 0 {
		t.Fatalf("Length(drained) = %d,%v want 0,nil", n, err)
	}

	if ok, err := s.IsEmpty(ctx, "jobs"); err != nil || !ok {
		t.Fatalf("IsEmpty(drained) = %v,%v want true,nil", ok, err)
	}

	if got := s.Name(); got == "" {
		t.Fatal("Name is empty, want kit-stub")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

// TestEdgeStubQueuePopEmpty reports empty on a fresh stub.
func TestEdgeStubQueuePopEmpty(t *testing.T) {
	t.Parallel()

	s := newStubQueue()

	if _, err := s.Pop(t.Context(), "missing"); err == nil {
		t.Fatal("Pop(empty) = nil, want ErrEmpty")
	}
}
