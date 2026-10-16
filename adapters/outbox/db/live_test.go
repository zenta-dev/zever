package db

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// liveTableSeq hands out unique table suffixes so reruns against a shared
// database never collide with residue from an earlier run (cleanup DROP
// runs on the already-canceled test context and best-effort fails).
var liveTableSeq atomic.Uint64

// TestPostgresLive exercises the driver end to end against a live postgres.
// Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresLive(t *testing.T) {
	dsn := postgresDSN(t)

	pub := &recordingPublisher{}

	n := liveTableSeq.Add(1)
	table := fmt.Sprintf("outbox_live_%d", n)
	inbox := fmt.Sprintf("inbox_live_%d", n)

	d := mustNew(t, Options{DSN: dsn, Table: table, InboxTable: inbox, Publisher: pub})

	ctx := t.Context()

	t.Cleanup(func() {
		// context.Background: t.Context is already canceled during cleanup.
		_, _ = d.conn.Exec(context.Background(), `DROP TABLE IF EXISTS "`+table+`"`)
		_, _ = d.conn.Exec(context.Background(), `DROP TABLE IF EXISTS "`+inbox+`"`)
	})

	mustRecord(t, d, outbox.Message{ID: "live-1", Topic: "orders", Payload: []byte("p")})

	d.pollOnce(ctx)

	got := pub.messages()
	if len(got) != 1 {
		t.Fatalf("published %d, want 1", len(got))
	}

	if st := d.Status(); st.Processed != 1 {
		t.Errorf("Status().Processed = %d, want 1", st.Processed)
	}

	calls := 0
	fn := func(context.Context, coredb.Tx) error {
		calls++

		return errNil
	}

	process := func() error {
		return coredb.WithTx(ctx, d.conn, nil, func(ctx context.Context, tx coredb.Tx) error {
			return d.Process(ctx, tx, "live-evt", fn)
		})
	}

	if err := process(); err != nil {
		t.Fatalf("Process(first) error = %v", err)
	}

	if err := process(); err != nil {
		t.Fatalf("Process(repeat) error = %v", err)
	}

	if calls != 1 {
		t.Errorf("inbox fn calls = %d, want 1", calls)
	}
}
