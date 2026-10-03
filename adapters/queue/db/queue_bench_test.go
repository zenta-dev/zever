package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// benchDriver opens a DB-backed queue over a private sqlite file and returns
// the concrete *driver so the benchmark can seed rows and call reclaimStale
// directly.
func benchDriver(b *testing.B) *driver {
	b.Helper()

	conn, err := dbsqlite.New(coredb.Options{Path: b.TempDir() + "/bench.db"})
	if err != nil {
		b.Fatalf("sqlite New failed: %v", err)
	}

	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	q, err := NewFromDB(conn, Options{Owner: "owner-a", ReclaimBatch: 100})
	if err != nil {
		b.Fatalf("NewFromDB failed: %v", err)
	}

	d, ok := q.(*driver)
	if !ok {
		b.Fatalf("NewFromDB returned %T, want *driver", q)
	}

	return d
}

// seedStaleRows inserts n rows holding an expired foreign lease on topic,
// through one multi-row INSERT.
func seedStaleRows(ctx context.Context, d *driver, n int) error {
	now := time.Now().UTC()
	ins := orm.InsertInto(d.tbl)

	for i := 0; i < n; i++ {
		ins = ins.Values(
			orm.Set(d.cID, fmt.Sprintf("11111111-1111-7111-8111-%012d", i)),
			orm.Set(d.cTopic, "jobs"),
			orm.Set(d.cPayload, []byte("work")),
			orm.Set(d.cHeaders, `{"k":"v"}`),
			orm.Set(d.cAttempt, int64(1)),
			orm.Set(d.cAvail, now.Add(-time.Minute)),
			orm.Set(d.cOwner, "owner-crashed"),
			orm.Set(d.cUntil, now.Add(-time.Minute)),
			orm.Set(d.cCreated, now.Add(-time.Minute)),
		)
	}

	return ins.Exec(ctx, d.conn)
}

// restaleRows returns the reclaimed rows to a stale state with one multi-row
// UPDATE so the next benchmark iteration has work to reclaim.
func restaleRows(ctx context.Context, d *driver) error {
	_, err := orm.UpdateTable(d.tbl).Where(d.cTopic.Eq("jobs")).Set(
		orm.Set(d.cOwner, "owner-crashed"),
		orm.Set(d.cUntil, time.Now().UTC().Add(-time.Minute)),
	).Exec(ctx, d.conn)

	return err
}

// BenchmarkReclaimStale measures the stale-claim sweep: one SELECT plus one
// set-based UPDATE for the whole batch (attempt+1 computed by the database
// under the lease guard). Each iteration reclaims a full batch of stale rows,
// then re-stales them with a single multi-row UPDATE so the next iteration
// has work. The re-stale is one statement against the reclaim's batch+1, so
// the reclaim dominates the measurement.
func BenchmarkReclaimStale(b *testing.B) {
	d := benchDriver(b)
	ctx := b.Context()

	if err := seedStaleRows(ctx, d, d.batch); err != nil {
		b.Fatalf("seed failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := d.reclaimStale(ctx, "jobs"); err != nil {
			b.Fatalf("reclaimStale failed: %v", err)
		}

		if err := restaleRows(ctx, d); err != nil {
			b.Fatalf("restale failed: %v", err)
		}
	}
}
