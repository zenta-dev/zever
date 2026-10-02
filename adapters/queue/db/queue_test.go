package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	queuedb "github.com/zenta-dev/zever/adapters/queue/db"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/orm/dialect"
)

func mustNew(t *testing.T, o queuedb.Options) queue.Queue {
	t.Helper()

	q, err := queuedb.New(o)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	return q
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		options queuedb.Options
		wantErr bool
	}{
		{name: "zero", options: queuedb.Options{}, wantErr: false},
		{name: "negative visibility", options: queuedb.Options{VisibilityTimeout: -time.Second}, wantErr: true},
		{name: "negative poll", options: queuedb.Options{PollTimeout: -time.Second}, wantErr: true},
		{name: "negative interval", options: queuedb.Options{PollInterval: -time.Second}, wantErr: true},
		{name: "negative batch", options: queuedb.Options{ReclaimBatch: -1}, wantErr: true},
		{name: "negative buffer", options: queuedb.Options{Buffer: -1}, wantErr: true},
		{name: "bad table", options: queuedb.Options{Table: "queue-messages"}, wantErr: true},
		{name: "good table", options: queuedb.Options{Table: "queue_messages_2"}, wantErr: false},
	}

	for _, tc := range cases {
		err := tc.options.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("Validate(%s) error = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestDBOptionsMapping(t *testing.T) {
	t.Parallel()

	q := mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "queue.db")},
		Table:   "custom_table",
	})

	ctx := t.Context()

	if err := q.Push(ctx, "t", queue.Payload("v"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	msg, err := q.Pop(ctx, "t")
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	if string(msg.Payload) != "v" {
		t.Errorf("Payload = %q, want v", msg.Payload)
	}

	if msg.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", msg.Attempt)
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
}

func TestClosed(t *testing.T) {
	t.Parallel()

	q := mustNew(t, queuedb.Options{
		Options: coredb.Options{Path: filepath.Join(t.TempDir(), "queue.db")},
	})

	ctx := t.Context()

	if err := q.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := q.Close(); err != nil {
		t.Fatalf("Close() second error = %v, want nil", err)
	}

	if err := q.Push(ctx, "t", queue.Payload("x"), nil); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Push() err = %v, want ErrClosed", err)
	}

	if _, err := q.Pop(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Pop() err = %v, want ErrClosed", err)
	}

	if _, err := q.Length(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Length() err = %v, want ErrClosed", err)
	}

	if _, err := q.IsEmpty(ctx, "t"); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("IsEmpty() err = %v, want ErrClosed", err)
	}

	msg := queue.NewMessage("t", queue.Payload("x"), nil)
	if err := q.Ack(ctx, msg); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Ack() err = %v, want ErrClosed", err)
	}

	if err := q.Nack(ctx, msg, true); !errors.Is(err, queue.ErrClosed) {
		t.Errorf("Nack() err = %v, want ErrClosed", err)
	}
}

func TestNewFromDB(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "fromdb.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	q, err := queuedb.NewFromDB(conn, queuedb.Options{Owner: "owner-fromdb"})
	if err != nil {
		t.Fatalf("NewFromDB failed: %v", err)
	}

	ctx := t.Context()

	if err := q.Push(ctx, "t", queue.Payload("v"), nil); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if _, err := q.Pop(ctx, "t"); err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	// Borrowed connection: driver Close must not close the injected DB.
	if err := q.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("injected DB closed by driver Close: %v", err)
	}
}

func TestNewFromDBNilDB(t *testing.T) {
	t.Parallel()

	if _, err := queuedb.NewFromDB(nil, queuedb.Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

// TestConcurrentPopDistinct proves parallel Pops serialize on the claim
// CAS: every message delivers exactly once across workers.
func TestConcurrentPopDistinct(t *testing.T) {
	t.Parallel()

	q := mustNew(t, queuedb.Options{
		Options:     coredb.Options{Path: filepath.Join(t.TempDir(), "queue.db")},
		PollTimeout: 300 * time.Millisecond,
	})

	ctx := t.Context()

	const total = 20

	for i := 0; i < total; i++ {
		if err := q.Push(ctx, "jobs", queue.Payload([]byte{byte(i)}), nil); err != nil {
			t.Fatalf("Push(%d) error = %v", i, err)
		}
	}

	var mu sync.Mutex

	seen := make(map[queue.MessageID]bool)

	var wg sync.WaitGroup

	for w := 0; w < 4; w++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				msg, err := q.Pop(ctx, "jobs")
				if err != nil {
					if errors.Is(err, queue.ErrEmpty) {
						return
					}

					t.Errorf("Pop() error = %v", err)

					return
				}

				mu.Lock()
				seen[msg.ID] = true
				mu.Unlock()

				if err := q.Ack(ctx, msg); err != nil {
					t.Errorf("Ack() error = %v", err)

					return
				}
			}
		}()
	}

	wg.Wait()

	if len(seen) != total {
		t.Fatalf("distinct deliveries = %d, want %d", len(seen), total)
	}

	if n, err := q.Length(ctx, "jobs"); err != nil || n != 0 {
		t.Fatalf("Length() = %d,%v want 0,nil", n, err)
	}
}

// stubDB is a test double reporting an unsupported dialect.
type stubDB struct{ coredb.DB }

func (s stubDB) Ping(context.Context) error { return nil }

func (s stubDB) Dialect() string { return "oracle" }

// TestUnsupportedDialectFailsClosed proves drivers over unknown dialects
// fail with ErrUnsupportedByDialect instead of running.
func TestUnsupportedDialectFailsClosed(t *testing.T) {
	t.Parallel()

	q, err := queuedb.NewFromDB(stubDB{}, queuedb.Options{})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("NewFromDB(oracle) err = %v (%T), want ErrUnsupportedByDialect", err, q)
	}
}
