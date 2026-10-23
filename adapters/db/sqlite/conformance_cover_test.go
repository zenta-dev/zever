package sqlite

import (
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/db/dbtest"
)

// TestOpenDirect proves direct construction opens a working database.
func TestOpenDirect(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = d.Close(t.Context()) }()

	if got := d.Dialect(); got != "sqlite" {
		t.Errorf("Dialect() = %q, want %q", got, "sqlite")
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
