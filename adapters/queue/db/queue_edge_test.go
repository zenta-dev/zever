package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
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

// TestOpenFromDB_borrowsConn proves OpenFromDB is a thin alias over NewFromDB:
// it opens over the caller's connection and Close leaves that connection
// usable (the caller retains ownership).
func TestOpenFromDB_borrowsConn(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "open.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	q, err := queuedb.OpenFromDB(conn, queuedb.Options{Owner: "owner-open"})
	if err != nil {
		t.Fatalf("OpenFromDB failed: %v", err)
	}

	if err := q.Push(ctx, "t", queue.Payload("v"), nil); err != nil {
		t.Fatalf("Push() = %v", err)
	}

	if _, err := q.Pop(ctx, "t"); err != nil {
		t.Fatalf("Pop() = %v", err)
	}

	if err := q.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("borrowed conn closed by driver Close: %v", err)
	}
}

// TestPush_bufferBlocksUntilPopFrees proves a bounded buffer gates Push: the
// second push cannot proceed while the topic holds Buffer ready messages, and
// a Pop freeing a slot unblocks it.
func TestPush_bufferBlocksUntilPopFrees(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "buf.db")},
		Buffer:  1,
	})

	if err := q.Push(ctx, "t", queue.Payload("one"), nil); err != nil {
		t.Fatalf("Push(one) = %v", err)
	}

	done := make(chan error, 1)

	go func() { done <- q.Push(ctx, "t", queue.Payload("two"), nil) }()

	msg, err := q.Pop(ctx, "t")
	if err != nil {
		t.Fatalf("Pop() = %v", err)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() = %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("blocked Push() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Push() stayed blocked after a buffer slot freed")
	}
}

// TestPush_bufferContextCancelled proves a blocked Push returns the context
// error wrapped so errors.Is sees it, instead of waiting forever.
func TestPush_bufferContextCancelled(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "buf.db")},
		Buffer:  1,
	})

	if err := q.Push(ctx, "t", queue.Payload("one"), nil); err != nil {
		t.Fatalf("Push(one) = %v", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	if err := q.Push(cctx, "t", queue.Payload("two"), nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Push(blocked) = %v, want DeadlineExceeded", err)
	}
}

// TestPushDelayed_bufferCountsUnclaimed proves the delayed buffer gate counts
// delayed rows too (unclaimed), so a full topic blocks a further delayed push
// until capacity frees.
func TestPushDelayed_bufferCountsUnclaimed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "buf.db")},
		Buffer:  1,
	})

	if err := q.PushDelayed(ctx, "t", queue.Payload("one"), nil, time.Minute); err != nil {
		t.Fatalf("PushDelayed(one) = %v", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	if err := q.PushDelayed(cctx, "t", queue.Payload("two"), nil, time.Minute); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PushDelayed(blocked) = %v, want DeadlineExceeded", err)
	}
}

// TestPushDelayed_immediateClaimable proves a non-positive delay makes the
// message immediately claimable instead of scheduling it.
func TestPushDelayed_immediateClaimable(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newEdgeQueue(t)

	if err := q.PushDelayed(ctx, "t", queue.Payload("now"), nil, 0); err != nil {
		t.Fatalf("PushDelayed(0) = %v", err)
	}

	msg, err := q.Pop(ctx, "t")
	if err != nil {
		t.Fatalf("Pop() = %v", err)
	}

	if string(msg.Payload) != "now" {
		t.Errorf("Payload = %q, want now", msg.Payload)
	}
}

// TestNack_unknownMessage_noop proves Nack on a message that never existed (or
// already settled) is a no-op, matching the redis script miss behavior.
func TestNack_unknownMessage_noop(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newEdgeQueue(t)
	msg := queue.NewMessage("t", queue.Payload("x"), nil)

	if err := q.Nack(ctx, msg, true); err != nil {
		t.Fatalf("Nack(unknown, requeue) = %v", err)
	}

	if err := q.Nack(ctx, msg, false); err != nil {
		t.Fatalf("Nack(unknown, drop) = %v", err)
	}
}

// TestOps_cancelledContext proves every context-taking operation fails closed
// with the caller's context error before touching the database.
func TestOps_cancelledContext(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	q := newEdgeQueue(t)

	cctx, cancel := context.WithCancel(ctx)
	cancel()

	msg := queue.NewMessage("t", queue.Payload("x"), nil)

	if err := q.Push(cctx, "t", queue.Payload("x"), nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Push(cancelled) = %v, want context.Canceled", err)
	}

	if err := q.PushDelayed(cctx, "t", queue.Payload("x"), nil, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("PushDelayed(cancelled) = %v, want context.Canceled", err)
	}

	if _, err := q.Pop(cctx, "t"); !errors.Is(err, context.Canceled) {
		t.Errorf("Pop(cancelled) = %v, want context.Canceled", err)
	}

	if err := q.Ack(cctx, msg); !errors.Is(err, context.Canceled) {
		t.Errorf("Ack(cancelled) = %v, want context.Canceled", err)
	}

	if err := q.Nack(cctx, msg, true); !errors.Is(err, context.Canceled) {
		t.Errorf("Nack(cancelled) = %v, want context.Canceled", err)
	}

	if _, err := q.Length(cctx, "t"); !errors.Is(err, context.Canceled) {
		t.Errorf("Length(cancelled) = %v, want context.Canceled", err)
	}
}

// TestRegister_opensViaCoreOptions proves Register wires the DB adapter into
// both core registries: queue.Open resolves the own-pool factory and
// queue.OpenShared resolves the borrowed-pool factory.
func TestRegister_opensViaCoreOptions(t *testing.T) {
	t.Parallel()

	queuedb.Register()

	ctx := t.Context()

	q, err := queue.Open(queue.DB, queue.Options{})
	if err != nil {
		t.Fatalf("queue.Open(db) = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	if q.Name() != "db" {
		t.Errorf("Name() = %q, want db", q.Name())
	}

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "shared.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	shared, err := queue.OpenShared(queue.DB, conn, queue.Options{})
	if err != nil {
		t.Fatalf("queue.OpenShared(db) = %v", err)
	}

	if err := shared.Push(ctx, "t", queue.Payload("v"), nil); err != nil {
		t.Fatalf("shared Push() = %v", err)
	}

	if _, err := shared.Pop(ctx, "t"); err != nil {
		t.Fatalf("shared Pop() = %v", err)
	}

	if err := shared.Close(); err != nil {
		t.Fatalf("shared Close() = %v", err)
	}
}
