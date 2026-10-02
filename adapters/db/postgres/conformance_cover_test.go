package postgres

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/db/dbtest"
)

// TestConformance runs the shared db kit against postgres via Register+Open.
// Skipped unless POSTGRES_DSN names a live server; unit coverage with dead
// DSNs lives in postgres_test.go. Never fake infra for conformance.
func TestConformance(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres conformance against a live server")
	}

	dbtest.Conformance(t, func(t *testing.T) db.DB {
		t.Helper()

		Register()

		d, err := db.Open(db.Postgres, db.Options{DSN: dsn})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}

		t.Cleanup(func() { _ = d.Close(t.Context()) })

		return d
	})
}
