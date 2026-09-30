package postgres

import (
	"os"
	"testing"
)

// TestPostgresLive_CreateRunToComplete exercises the driver end to end
// against a live postgres. Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresLive_CreateRunToComplete(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	d := mustNew(t, Options{DSN: dsn, Table: "workflow_runs_live", Owner: "owner-live"})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	t.Cleanup(func() {
		_, _ = d.conn.Exec(ctx, `DROP TABLE "workflow_runs_live"`)
	})

	id, err := d.Start(ctx, "greet", "hello", "live-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var out string
	if err := d.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "hello" {
		t.Fatalf("state = %q, want %q", out, "hello")
	}
}
