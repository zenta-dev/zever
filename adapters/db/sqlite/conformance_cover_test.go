package sqlite

import (
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/db/dbtest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name: registering db.SQLite itself would collide with
// ExampleOpen's strict Register in this same binary (see the note atop
// sqlite_test.go), so the kit factory below opens via New directly.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = db.Adapter("kit-db-sqlite")

	if err := db.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	d, err := db.Open(adapter, db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = d.Close(t.Context()) }()

	if got := d.Dialect(); got != db.SQLite.String() {
		t.Errorf("Dialect() = %q, want %q", got, db.SQLite.String())
	}

	if err := d.Ping(t.Context()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// TestConformance runs the shared db kit against sqlite.
func TestConformance(t *testing.T) {
	t.Parallel()

	dbtest.Conformance(t, func(t *testing.T) db.DB {
		t.Helper()

		d, err := New(db.Options{Path: ":memory:"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = d.Close(t.Context()) })

		return d
	})
}
