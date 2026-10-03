package db

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/shared/dbconn"
)

func mustDriver(t *testing.T, o Options) *driver {
	t.Helper()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "internal.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	q, err := NewFromDB(conn, o)
	if err != nil {
		t.Fatalf("NewFromDB failed: %v", err)
	}

	d, ok := q.(*driver)
	if !ok {
		t.Fatalf("NewFromDB returned %T, want *driver", q)
	}

	return d
}

// TestReclaimStaleBumpsAttempt inserts a row holding an expired foreign
// lease and proves the sweep releases it with attempt+1 without touching
// the stored payload.
func TestReclaimStaleBumpsAttempt(t *testing.T) {
	t.Parallel()

	d := mustDriver(t, Options{Owner: "owner-a"})
	ctx := t.Context()
	now := time.Now().UTC()

	if err := orm.InsertInto(d.tbl).Values(
		orm.Set(d.cID, "11111111-1111-7111-8111-111111111111"),
		orm.Set(d.cTopic, "jobs"),
		orm.Set(d.cPayload, []byte("work")),
		orm.Set(d.cHeaders, `{"k":"v"}`),
		orm.Set(d.cAttempt, int64(1)),
		orm.Set(d.cAvail, now.Add(-time.Minute)),
		orm.Set(d.cOwner, "owner-crashed"),
		orm.Set(d.cUntil, now.Add(-time.Minute)),
		orm.Set(d.cCreated, now.Add(-time.Minute)),
	).Exec(ctx, d.conn); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if err := d.reclaimStale(ctx, "jobs"); err != nil {
		t.Fatalf("reclaimStale failed: %v", err)
	}

	row, ok, err := orm.From[msgRow, *msgRow](d.tbl).Where(
		d.cID.Eq("11111111-1111-7111-8111-111111111111"),
	).First(ctx, d.conn)
	if err != nil || !ok {
		t.Fatalf("load = %+v,%v want row,nil", row, err)
	}

	if row.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", row.Attempt)
	}

	if row.ClaimedBy != "" {
		t.Errorf("ClaimedBy = %q, want empty (released)", row.ClaimedBy)
	}

	if string(row.Payload) != "work" {
		t.Errorf("Payload = %q, want work (untouched)", row.Payload)
	}
}

// TestReclaimStaleCapsBatch inserts batch+5 stale rows and proves one sweep
// releases at most batch of them: the SELECT's LIMIT and the id IN(...) guard
// together keep the set-based UPDATE inside the batch bound.
func TestReclaimStaleCapsBatch(t *testing.T) {
	t.Parallel()

	d := mustDriver(t, Options{Owner: "owner-a", ReclaimBatch: 10})
	ctx := t.Context()

	if err := seedStaleRows(ctx, d, d.batch+5); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := d.reclaimStale(ctx, "jobs"); err != nil {
		t.Fatalf("reclaimStale failed: %v", err)
	}

	ready, err := orm.From[msgRow, *msgRow](d.tbl).Where(orm.And(
		d.cTopic.Eq("jobs"),
		d.cOwner.Eq(""),
	)).All(ctx, d.conn)
	if err != nil {
		t.Fatalf("count ready: %v", err)
	}

	if len(ready) != d.batch {
		t.Fatalf("released %d rows, want %d (batch cap)", len(ready), d.batch)
	}
}

// TestReclaimStaleSingleSetBasedUpdate proves the sweep collapsed to ONE
// set-based UPDATE: the captured statements contain no per-row UPDATE, and
// the single UPDATE computes attempt+1 in the database under the lease guard
// (owner still set, claim still unexpired) scoped to the selected ids.
func TestReclaimStaleSingleSetBasedUpdate(t *testing.T) {
	t.Parallel()

	d := mustDriver(t, Options{Owner: "owner-a"})
	ctx := t.Context()

	if err := seedStaleRows(ctx, d, 3); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	var (
		mu      sync.Mutex
		queries []string
	)

	restore := orm.SetQueryLogger(func(query string, _ []any) {
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()
	})
	defer restore()

	if err := d.reclaimStale(ctx, "jobs"); err != nil {
		t.Fatalf("reclaimStale failed: %v", err)
	}

	// The logger is process-wide and parallel tests share it (Pop runs the
	// same sweep), so assert on statement SHAPES, not counts: the set-based
	// UPDATE carries the lease guard and the id IN(...) scope, and no
	// per-row guarded UPDATE (the old N+1 shape) ran.
	sawSetBased := false

	for _, q := range queries {
		switch {
		case strings.HasPrefix(q, `UPDATE "queue_messages" SET "attempt" = ?`):
			t.Errorf("per-row guarded UPDATE ran (old N+1 shape): %q", q)
		case strings.HasPrefix(q, `UPDATE "queue_messages" SET "attempt" = "attempt" + ?`):
			sawSetBased = true

			for _, want := range []string{
				`"topic" = ?`,
				`"claimed_by" != ?`,
				`"claimed_until" <= ?`,
				`"message_id" IN (`,
			} {
				if !strings.Contains(q, want) {
					t.Errorf("UPDATE %q missing %q", q, want)
				}
			}
		}
	}

	if !sawSetBased {
		t.Errorf("no set-based reclaim UPDATE captured: %q", queries)
	}
}

// TestTryClaimContention proves two drivers racing one ready row serialize:
// exactly one wins the CAS and the loser reports not-claimed.
func TestTryClaimContention(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared.db")

	newShared := func(t *testing.T, owner string) *driver {
		t.Helper()

		conn, err := dbsqlite.New(coredb.Options{Path: path})
		if err != nil {
			t.Fatalf("sqlite New failed: %v", err)
		}

		t.Cleanup(func() { _ = conn.Close(t.Context()) })

		q, err := NewFromDB(conn, Options{Owner: owner})
		if err != nil {
			t.Fatalf("NewFromDB failed: %v", err)
		}

		d, ok := q.(*driver)
		if !ok {
			t.Fatalf("NewFromDB returned %T, want *driver", q)
		}

		return d
	}

	a := newShared(t, "owner-a")
	b := newShared(t, "owner-b")
	ctx := t.Context()

	if err := a.Push(ctx, "jobs", queue.Payload("work"), nil); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	msgA, okA, err := a.tryClaim(ctx, "jobs")
	if err != nil {
		t.Fatalf("tryClaim(a) failed: %v", err)
	}

	_, okB, err := b.tryClaim(ctx, "jobs")
	if err != nil {
		t.Fatalf("tryClaim(b) failed: %v", err)
	}

	if !okA {
		t.Fatal("tryClaim(a) = not-claimed, want claimed")
	}

	if okB {
		t.Fatal("tryClaim(b) = claimed, want not-claimed (a holds the lease)")
	}

	if string(msgA.Payload) != "work" {
		t.Errorf("Payload = %q, want work", msgA.Payload)
	}
}

// TestIsPostgresDSN pins the DSN selection rule shared with the search
// fold: postgres URLs open postgres, everything else is a sqlite path.
func TestIsPostgresDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dsn  string
		want bool
	}{
		{"postgres://user:pass@localhost:5432/q?sslmode=disable", true},
		{"postgresql://localhost/q", true},
		{"  POSTGRES://localhost/q  ", true},
		{"", false},
		{":memory:", false},
		{"/data/queue.db", false},
		{"queue.db", false},
	}

	for _, tc := range cases {
		if got := dbconn.IsPostgresDSN(tc.dsn); got != tc.want {
			t.Errorf("IsPostgresDSN(%q) = %v, want %v", tc.dsn, got, tc.want)
		}
	}
}
