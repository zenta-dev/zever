package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

func TestMemory_Push_emptyTopic_roundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newQueue(t, queue.Options{})

	if err := q.Push(ctx, "", queue.Payload([]byte("v")), nil); err != nil {
		t.Fatalf("Push(empty topic) = %v", err)
	}
	msg, err := q.Pop(ctx, "")
	if err != nil {
		t.Fatalf("Pop(empty topic) = %v", err)
	}
	if string(msg.Payload) != "v" {
		t.Errorf("Payload = %q, want v", msg.Payload)
	}
	if err := q.Ack(ctx, msg); err != nil {
		t.Errorf("Ack() = %v", err)
	}
}

func TestMemory_Push_nilPayload_roundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newQueue(t, queue.Options{})

	if err := q.Push(ctx, "nil-payload", nil, nil); err != nil {
		t.Fatalf("Push(nil payload) = %v", err)
	}
	msg, err := q.Pop(ctx, "nil-payload")
	if err != nil {
		t.Fatalf("Pop() = %v", err)
	}
	if len(msg.Payload) != 0 {
		t.Errorf("Payload = %q, want empty", msg.Payload)
	}
}

// TestRegister_opensViaCoreOptions proves Register wires the memory adapter
// into the core registry so queue.Open resolves it.
func TestRegister_opensViaCoreOptions(t *testing.T) {

	q, err := New(queue.Options{})
	if err != nil {
		t.Fatalf("queue.Open(memory) = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	if q.Name() != "memory" {
		t.Errorf("Name() = %q, want memory", q.Name())
	}
}

func TestMemory_Length_unknownTopicZero(t *testing.T) {
	t.Parallel()

	q := newQueue(t, queue.Options{})
	n, err := q.Length(t.Context(), "never-used")
	if err != nil {
		t.Fatalf("Length(unknown) = %v", err)
	}
	if n != 0 {
		t.Errorf("Length(unknown) = %d, want 0", n)
	}
	empty, err := q.IsEmpty(t.Context(), "never-used")
	if err != nil {
		t.Fatalf("IsEmpty(unknown) = %v", err)
	}
	if !empty {
		t.Error("IsEmpty(unknown) = false, want true")
	}
}

func TestDelayedHeapOrder(t *testing.T) {
	t.Parallel()

	tq := newTopicQueue()
	base := time.Now().Add(-time.Hour)
	offsets := []int{5, 1, 9, 3, 7, 2, 8, 0, 6, 4, 3}

	for _, o := range offsets {
		tq.pushDelayedLocked(delayedEntry{
			msg:     queue.NewMessage("t", queue.Payload([]byte("x")), nil),
			readyAt: base.Add(time.Duration(o) * time.Second),
		})
	}

	due, _, hasMore := tq.collectDue()
	if hasMore {
		t.Fatalf("hasMore = true, want false")
	}

	if len(due) != len(offsets) {
		t.Fatalf("due len = %d, want %d", len(due), len(offsets))
	}

	for i := 1; i < len(due); i++ {
		if due[i].readyAt.Before(due[i-1].readyAt) {
			t.Fatalf("due[%d] before due[%d]: not sorted", i, i-1)
		}
	}
}

func TestDelayedHeapNextDue(t *testing.T) {
	t.Parallel()

	tq := newTopicQueue()
	now := time.Now()
	soon, later := now.Add(time.Hour), now.Add(2*time.Hour)

	for _, at := range []time.Time{later, now.Add(-time.Second), soon} {
		tq.pushDelayedLocked(delayedEntry{msg: queue.NewMessage("t", queue.Payload([]byte("x")), nil), readyAt: at})
	}

	due, next, hasMore := tq.collectDue()
	if len(due) != 1 || !hasMore || !next.Equal(soon) {
		t.Fatalf("collectDue = %d due, next=%v, more=%v; want 1, %v, true", len(due), next, hasMore, soon)
	}
}
