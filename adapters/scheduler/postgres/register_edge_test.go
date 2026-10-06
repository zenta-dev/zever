package postgres

import (
	"context"
	"path/filepath"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
)

// TestRegister proves Register wires the postgres adapter's DSN factory into
// the scheduler registry, so Open resolves it to a live scheduler.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	opts := scheduler.Options{
		Dispatcher: &job.Dispatcher{Q: newStubQueue()},
		DSN:        filepath.Join(t.TempDir(), "register.db"),
	}

	s, err := scheduler.Open(Adapter, opts)
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", Adapter, err)
	}

	if got := s.Name(); got != "postgres" {
		t.Fatalf("Name() = %q, want postgres", got)
	}

	if c, ok := s.(interface{ Close() error }); ok {
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}

// TestRegisterShared proves the shared-pool factory registered by Register
// builds a scheduler over a borrowed connection.
func TestRegisterShared(t *testing.T) {
	t.Parallel()

	Register()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "shared.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	opts := scheduler.Options{Dispatcher: &job.Dispatcher{Q: newStubQueue()}}

	s, err := scheduler.OpenShared(Adapter, conn, opts)
	if err != nil {
		t.Fatalf("OpenShared(%s): %v", Adapter, err)
	}

	if got := s.Name(); got != "postgres" {
		t.Fatalf("Name() = %q, want postgres", got)
	}

	if c, ok := s.(interface{ Close() error }); ok {
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
