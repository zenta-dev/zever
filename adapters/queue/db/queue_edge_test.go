package db_test

import (
	"path/filepath"
	"testing"

	queuedb "github.com/zenta-dev/zever/adapters/queue/db"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
)

func newEdgeQueue(t *testing.T) queue.Queue {
	t.Helper()
	return mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "edge.db")},
	})
}

func TestPush_nilPayload_rejected(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newEdgeQueue(t)

	// The schema stores payload NOT NULL, so a nil payload fails closed
	// rather than silently persisting an empty row.
	if err := q.Push(ctx, "t", nil, nil); err == nil {
		t.Fatal("Push(nil payload) = nil, want NOT NULL constraint error")
	}

	// The queue remains usable after the rejected push.
	if err := q.Push(ctx, "t", queue.Payload("v"), nil); err != nil {
		t.Fatalf("Push(valid) = %v", err)
	}
	if _, err := q.Pop(ctx, "t"); err != nil {
		t.Fatalf("Pop() = %v", err)
	}
}

func TestLength_unknownTopicZero(t *testing.T) {
	t.Parallel()

	q := newEdgeQueue(t)
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

func TestIsEmpty_emptyTopic(t *testing.T) {
	t.Parallel()

	q := newEdgeQueue(t)
	empty, err := q.IsEmpty(t.Context(), "")
	if err != nil {
		t.Fatalf("IsEmpty(empty topic) = %v", err)
	}
	if !empty {
		t.Error("IsEmpty(empty topic) = false, want true")
	}
}
