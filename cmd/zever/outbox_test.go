package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// outboxTestTSLayout mirrors the db adapter's fixed-width UTC timestamp
// layout so seeded rows sort chronologically.
const outboxTestTSLayout = "2006-01-02T15:04:05.000000000Z07:00"

// outboxCLIFixture is a fresh named in-memory sqlite outbox with one row of
// each status. The seed store stays open for the test's lifetime so the
// shared-cache in-memory database survives the per-command open/close cycles.
type outboxCLIFixture struct {
	dsn  string
	conn coredb.DB
}

// newOutboxCLIFixture builds the fixture and seeds pending/processed/failed
// rows. Each test gets a uniquely named in-memory database, so tests never
// collide through the process-global sqlite shared cache.
func newOutboxCLIFixture(t *testing.T, name string) *outboxCLIFixture {
	t.Helper()
	t.Chdir(t.TempDir())

	prevJSON := jsonMode
	jsonMode = false
	t.Cleanup(func() { jsonMode = prevJSON })

	dsn := "file:outboxtest_" + name + "?mode=memory&cache=shared"

	store, err := outbox.Open(outbox.DB, outbox.Options{DSN: dsn, Table: "outbox"})
	if err != nil {
		t.Fatalf("open outbox store: %v", err)
	}

	t.Cleanup(func() { _ = store.Close() })

	conn, err := dbsqlite.New(coredb.Options{Path: dsn})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	f := &outboxCLIFixture{dsn: dsn, conn: conn}
	base := time.Now().UTC().Add(-2 * time.Hour)

	f.seed(t, "pending-1", "pending", 0, base, time.Time{})
	f.seed(t, "processed-old", "processed", 1, base, base.Add(time.Minute))
	f.seed(t, "processed-new", "processed", 1, time.Now().UTC(), time.Now().UTC())
	f.seed(t, "failed-1", "failed", 3, base.Add(time.Minute), time.Time{})

	return f
}

// seed inserts one outbox row directly, bypassing the relay.
func (f *outboxCLIFixture) seed(t *testing.T, id, status string, attempts int, createdAt, processedAt time.Time) {
	t.Helper()

	var proc any
	if !processedAt.IsZero() {
		proc = processedAt.UTC().Format(outboxTestTSLayout)
	}

	_, err := f.conn.Exec(t.Context(),
		`INSERT INTO "outbox" (id, topic, "key", payload, headers, created_at, attempts, processed_at, status) `+
			`VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "topic."+id, "", []byte("payload"), "{}",
		createdAt.UTC().Format(outboxTestTSLayout), attempts, proc, status)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// statusOf returns the stored status for id, or "" when absent.
func (f *outboxCLIFixture) statusOf(t *testing.T, id string) string {
	t.Helper()

	rows, err := f.conn.Query(t.Context(), `SELECT status FROM "outbox" WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return ""
	}

	var s string
	if err := rows.Scan(&s); err != nil {
		t.Fatalf("scan status: %v", err)
	}

	return s
}

// TestOutboxUsage pins the help text and the dispatcher error contract.
func TestOutboxUsage(t *testing.T) {
	out := captureZeverStderr(t, printOutboxUsage)

	for _, want := range []string{"status", "dlq", "purge", "--dsn", "--before"} {
		if !strings.Contains(out, want) {
			t.Errorf("outbox usage missing %q:\n%s", want, out)
		}
	}

	if err := runOutbox(nil); !errors.Is(err, errOutboxUnknownSubcommand) {
		t.Fatalf("runOutbox(nil) = %v, want errOutboxUnknownSubcommand", err)
	}

	if err := runOutbox([]string{"--help"}); err != nil {
		t.Fatalf("runOutbox(--help) = %v, want nil", err)
	}

	if err := runOutbox([]string{"bogus"}); err == nil {
		t.Fatal("runOutbox(bogus) = nil, want error")
	}

	if err := runOutbox([]string{"dlq"}); !errors.Is(err, errOutboxUnknownSubcommand) {
		t.Fatalf("runOutbox(dlq) = %v, want errOutboxUnknownSubcommand", err)
	}

	if err := runOutbox([]string{"dlq", "bogus"}); err == nil {
		t.Fatal("runOutbox(dlq bogus) = nil, want error")
	}
}

// TestOutboxStatusCommand asserts the human status block reports every
// counter from the seeded rows.
func TestOutboxStatusCommand(t *testing.T) {
	f := newOutboxCLIFixture(t, "status")

	out := captureZeverStdout(t, func() {
		if err := runOutbox([]string{"status", "--dsn", f.dsn}); err != nil {
			t.Fatalf("outbox status: %v", err)
		}
	})

	for _, want := range []string{"pending", "processed", "failed", "table=outbox"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

// TestOutboxStatusJSON asserts --json emits the structured counters on stdout.
func TestOutboxStatusJSON(t *testing.T) {
	f := newOutboxCLIFixture(t, "statusjson")

	var buf bytes.Buffer

	old := jsonOut
	jsonOut = &buf
	jsonMode = true

	t.Cleanup(func() { jsonOut = old })

	if err := runOutbox([]string{"status", "--dsn", f.dsn}); err != nil {
		t.Fatalf("outbox status --json: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &env); err != nil {
		t.Fatalf("status JSON not an envelope: %v", err)
	}

	if !env.OK || env.Command != "outbox status" {
		t.Fatalf("envelope = %+v, want ok outbox status", env)
	}

	raw, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	var st outboxStatusData
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("data not status shape: %v", err)
	}

	if st.Pending != 1 || st.Processed != 2 || st.Failed != 1 {
		t.Fatalf("counts = %+v, want pending=1 processed=2 failed=1", st)
	}
}

// TestOutboxDLQList asserts the list shows failed rows only and honors
// --limit.
func TestOutboxDLQList(t *testing.T) {
	f := newOutboxCLIFixture(t, "dlqlist")
	f.seed(t, "failed-2", "failed", 5, time.Now().UTC(), time.Time{})

	out := captureZeverStdout(t, func() {
		if err := runOutbox([]string{"dlq", "list", "--dsn", f.dsn}); err != nil {
			t.Fatalf("dlq list: %v", err)
		}
	})

	if !strings.Contains(out, "failed-1") || !strings.Contains(out, "failed-2") {
		t.Errorf("dlq list missing failed rows:\n%s", out)
	}

	if strings.Contains(out, "pending-1") || strings.Contains(out, "processed-new") {
		t.Errorf("dlq list must only show failed rows:\n%s", out)
	}

	limited := captureZeverStdout(t, func() {
		if err := runOutbox([]string{"dlq", "list", "--limit", "1", "--dsn", f.dsn}); err != nil {
			t.Fatalf("dlq list --limit: %v", err)
		}
	})

	if strings.Contains(limited, "failed-2") {
		t.Errorf("dlq list --limit 1 should cap at one row:\n%s", limited)
	}
}

// TestOutboxDLQRequeue asserts --id and --all move failed rows to pending.
func TestOutboxDLQRequeue(t *testing.T) {
	f := newOutboxCLIFixture(t, "requeue")

	if err := runOutbox([]string{"dlq", "requeue", "--id", "failed-1", "--dsn", f.dsn}); err != nil {
		t.Fatalf("dlq requeue --id: %v", err)
	}

	if got := f.statusOf(t, "failed-1"); got != "pending" {
		t.Errorf("failed-1 status = %q, want pending", got)
	}

	f.seed(t, "failed-2", "failed", 5, time.Now().UTC(), time.Time{})

	if err := runOutbox([]string{"dlq", "requeue", "--all", "--dsn", f.dsn}); err != nil {
		t.Fatalf("dlq requeue --all: %v", err)
	}

	if got := f.statusOf(t, "failed-2"); got != "pending" {
		t.Errorf("failed-2 status = %q, want pending after --all", got)
	}

	if err := runOutbox([]string{"dlq", "requeue", "--dsn", f.dsn}); err == nil {
		t.Fatal("dlq requeue without --id/--all must error")
	}

	if err := runOutbox([]string{"dlq", "requeue", "--id", "failed-1", "--all", "--dsn", f.dsn}); err == nil {
		t.Fatal("dlq requeue with both --id and --all must error")
	}
}

// TestOutboxDLQPurge asserts --id deletes one failed row and reports a
// missing id as ErrNotFound.
func TestOutboxDLQPurge(t *testing.T) {
	f := newOutboxCLIFixture(t, "dlqpurge")

	if err := runOutbox([]string{"dlq", "purge", "--id", "failed-1", "--dsn", f.dsn}); err != nil {
		t.Fatalf("dlq purge --id: %v", err)
	}

	if got := f.statusOf(t, "failed-1"); got != "" {
		t.Errorf("failed-1 status = %q, want deleted", got)
	}

	if err := runOutbox([]string{"dlq", "purge", "--id", "missing", "--dsn", f.dsn}); !errors.Is(err, outbox.ErrNotFound) {
		t.Fatalf("dlq purge missing = %v, want ErrNotFound", err)
	}

	if err := runOutbox([]string{"dlq", "purge", "--dsn", f.dsn}); err == nil {
		t.Fatal("dlq purge without --id must error")
	}
}

// TestOutboxPurge asserts --before deletes only old processed rows.
func TestOutboxPurge(t *testing.T) {
	f := newOutboxCLIFixture(t, "purge")

	out := captureZeverStdout(t, func() {
		if err := runOutbox([]string{"purge", "--before", "1h", "--dsn", f.dsn}); err != nil {
			t.Fatalf("outbox purge: %v", err)
		}
	})

	if !strings.Contains(out, "purged 1") {
		t.Errorf("purge output = %q, want purged 1", out)
	}

	if got := f.statusOf(t, "processed-old"); got != "" {
		t.Errorf("processed-old status = %q, want deleted", got)
	}

	if got := f.statusOf(t, "processed-new"); got != "processed" {
		t.Errorf("processed-new status = %q, want retained", got)
	}

	if err := runOutbox([]string{"purge", "--dsn", f.dsn}); err == nil {
		t.Fatal("purge without --before must error")
	}

	if err := runOutbox([]string{"purge", "--before", "nonsense", "--dsn", f.dsn}); err == nil {
		t.Fatal("purge with invalid --before must error")
	}
}
