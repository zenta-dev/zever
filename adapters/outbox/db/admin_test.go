package db

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

// mustSeed inserts a row with the given status directly, bypassing the relay,
// so admin tests can exercise every status without a Publisher.
func mustSeed(t *testing.T, d *driver, id, status string, attempts int, createdAt, processedAt time.Time) {
	t.Helper()

	var proc any
	if !processedAt.IsZero() {
		proc = d.ts(processedAt)
	}

	_, err := d.conn.Exec(t.Context(),
		`INSERT INTO `+quoteIdent(d.table)+` (id, topic, "key", payload, headers, created_at, attempts, processed_at, status) `+
			`VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "topic."+id, "k", []byte("payload"), "{}", d.ts(createdAt), attempts, proc, status)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// statusOf returns the stored status for id, or "" when the row is absent.
func statusOf(t *testing.T, d *driver, id string) string {
	t.Helper()

	rows, err := d.conn.Query(t.Context(), `SELECT status FROM `+quoteIdent(d.table)+` WHERE id = ?`, id)
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

// attemptsOf returns the stored attempt count for id.
func attemptsOf(t *testing.T, d *driver, id string) int {
	t.Helper()

	rows, err := d.conn.Query(t.Context(), `SELECT attempts FROM `+quoteIdent(d.table)+` WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("query attempts: %v", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatalf("no row %q", id)
	}

	var n int
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("scan attempts: %v", err)
	}

	return n
}

// TestAdminList covers the default-failed filter, ordering, and the explicit
// status override.
func TestAdminList(t *testing.T) {
	d := mustNew(t, Options{})
	base := time.Now().UTC().Add(-time.Hour)

	mustSeed(t, d, "f-old", statusFailed, 3, base, time.Time{})
	mustSeed(t, d, "f-new", statusFailed, 3, base.Add(time.Minute), time.Time{})
	mustSeed(t, d, "p", statusPending, 0, base, time.Time{})
	mustSeed(t, d, "ok", statusProcessed, 1, base, base.Add(time.Minute))

	msgs, err := d.List(t.Context(), "", 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("List() len = %d, want 2", len(msgs))
	}

	if msgs[0].ID != "f-old" || msgs[1].ID != "f-new" {
		t.Fatalf("List() order = %q,%q, want f-old,f-new", msgs[0].ID, msgs[1].ID)
	}

	if msgs[0].Attempts != 3 {
		t.Errorf("List()[0].Attempts = %d, want 3", msgs[0].Attempts)
	}

	if msgs[0].Topic != "topic.f-old" {
		t.Errorf("List()[0].Topic = %q, want topic.f-old", msgs[0].Topic)
	}

	pending, err := d.List(t.Context(), statusPending, 0)
	if err != nil {
		t.Fatalf("List(pending) error = %v", err)
	}

	if len(pending) != 1 || pending[0].ID != "p" {
		t.Fatalf("List(pending) = %+v, want just p", pending)
	}

	limited, err := d.List(t.Context(), statusFailed, 1)
	if err != nil {
		t.Fatalf("List(limit 1) error = %v", err)
	}

	if len(limited) != 1 {
		t.Fatalf("List(limit 1) len = %d, want 1", len(limited))
	}
}

// TestAdminRequeue covers single-id and all-failed requeue.
func TestAdminRequeue(t *testing.T) {
	d := mustNew(t, Options{})
	base := time.Now().UTC()

	mustSeed(t, d, "f1", statusFailed, 3, base, time.Time{})
	mustSeed(t, d, "f2", statusFailed, 4, base, time.Time{})
	mustSeed(t, d, "p1", statusPending, 0, base, time.Time{})

	if err := d.Requeue(t.Context(), "f1"); err != nil {
		t.Fatalf("Requeue(f1) error = %v", err)
	}

	if got := statusOf(t, d, "f1"); got != statusPending {
		t.Errorf("f1 status = %q, want pending", got)
	}

	if got := attemptsOf(t, d, "f1"); got != 0 {
		t.Errorf("f1 attempts = %d, want 0", got)
	}

	if got := statusOf(t, d, "f2"); got != statusFailed {
		t.Errorf("f2 status = %q, want still failed", got)
	}

	if err := d.Requeue(t.Context(), ""); err != nil {
		t.Fatalf("Requeue(all) error = %v", err)
	}

	if got := statusOf(t, d, "f2"); got != statusPending {
		t.Errorf("f2 status = %q, want pending after requeue all", got)
	}

	if got := statusOf(t, d, "p1"); got != statusPending {
		t.Errorf("p1 status = %q, want pending (untouched)", got)
	}
}

// TestAdminPurge covers deletion of only old processed rows.
func TestAdminPurge(t *testing.T) {
	d := mustNew(t, Options{})
	now := time.Now().UTC()

	mustSeed(t, d, "old", statusProcessed, 1, now.Add(-2*time.Hour), now.Add(-2*time.Hour+time.Minute))
	mustSeed(t, d, "new", statusProcessed, 1, now, now)
	mustSeed(t, d, "fail", statusFailed, 1, now, time.Time{})

	n, err := d.Purge(t.Context(), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}

	if n != 1 {
		t.Fatalf("Purge() deleted = %d, want 1", n)
	}

	if got := statusOf(t, d, "old"); got != "" {
		t.Errorf("old still present with status %q", got)
	}

	if got := statusOf(t, d, "new"); got != statusProcessed {
		t.Errorf("new status = %q, want processed", got)
	}

	if got := statusOf(t, d, "fail"); got != statusFailed {
		t.Errorf("fail status = %q, want failed", got)
	}
}

// TestAdminDelete covers per-message deletion and the not-found path.
func TestAdminDelete(t *testing.T) {
	d := mustNew(t, Options{})
	mustSeed(t, d, "f1", statusFailed, 3, time.Now().UTC(), time.Time{})

	if err := d.Delete(t.Context(), "f1"); err != nil {
		t.Fatalf("Delete(f1) error = %v", err)
	}

	if got := statusOf(t, d, "f1"); got != "" {
		t.Errorf("f1 still present with status %q", got)
	}

	if err := d.Delete(t.Context(), "missing"); !errors.Is(err, outbox.ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
}
