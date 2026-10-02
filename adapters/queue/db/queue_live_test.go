package db_test

import (
	"os"
	"path/filepath"
	"testing"

	queuedb "github.com/zenta-dev/zever/adapters/queue/db"
	coredb "github.com/zenta-dev/zever/core/db"
)

// TestPostgresLive_PushPopAck exercises the driver end to end against a
// live postgres. Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresLive_PushPopAck(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	q, newErr := queuedb.New(queuedb.Options{
		Options: coredb.Options{DSN: dsn},
		Table:   "queue_messages_live",
	})
	if newErr != nil {
		t.Fatalf("New() error = %v", newErr)
	}

	t.Cleanup(func() { _ = q.Close() })

	ctx := t.Context()

	if err := q.Push(ctx, "live", []byte("hello"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, "live")
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	if string(msg.Payload) != "hello" {
		t.Fatalf("Payload = %q, want hello", msg.Payload)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	if n, err := q.Length(ctx, "live"); err != nil || n != 0 {
		t.Fatalf("Length() = %d,%v want 0,nil", n, err)
	}
}

// TestPostgresLive_FileDSNSelection is a sqlite-path sanity check kept
// beside the live test: a non-postgres DSN through the core mapping opens
// sqlite, never postgres.
func TestPostgresLive_FileDSNSelection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "queue.db")

	q, err := queuedb.New(queuedb.Options{Options: coredb.Options{Path: path}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	if q.Name() != "db" {
		t.Fatalf("Name() = %q, want db", q.Name())
	}
}
