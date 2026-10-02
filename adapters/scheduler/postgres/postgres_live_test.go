package postgres

import (
	"os"
	"testing"
)

// TestPostgresLive_LeaseRecovery exercises the driver end to end against
// a live postgres. Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresLive_LeaseRecovery(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	registerJobOnce(t, "sched-live")

	d := mustNew(t, Options{
		PoolOptions: poolWithDSN(dsn),
		Owner:       "owner-live",
		Table:       "scheduler_slots_live",
	})
	t.Cleanup(func() { _ = d.Close() })

	ctx := t.Context()

	t.Cleanup(func() {
		_, _ = d.conn.Exec(ctx, `DROP TABLE "scheduler_slots_live"`)
	})

	id, err := d.Schedule(ctx, "0 * * * *", "sched-live", "hello")
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	if len(d.Entries()) != 1 {
		t.Fatalf("Entries = %d, want 1", len(d.Entries()))
	}

	if err := d.Remove(id); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
}
