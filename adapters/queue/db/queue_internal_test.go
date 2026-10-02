package db

import (
	"path/filepath"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/orm"
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
		if got := isPostgresDSN(tc.dsn); got != tc.want {
			t.Errorf("isPostgresDSN(%q) = %v, want %v", tc.dsn, got, tc.want)
		}
	}
}
