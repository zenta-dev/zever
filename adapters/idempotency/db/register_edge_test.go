package db

import (
	"context"
	"path/filepath"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/idempotency"
)

// TestRegister proves Register wires the db adapter's DSN factory into the
// idempotency registry, so Open resolves it to a live store.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	s, err := idempotency.Open(Adapter, idempotency.Options{DSN: filepath.Join(t.TempDir(), "register.db")})
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", Adapter, err)
	}

	if s == nil {
		t.Fatal("Open returned nil store")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestRegisterShared proves the shared-pool factory registered by Register
// builds a store over a borrowed connection.
func TestRegisterShared(t *testing.T) {
	t.Parallel()

	Register()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "register-shared.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	s, err := idempotency.OpenShared(Adapter, conn, idempotency.Options{})
	if err != nil {
		t.Fatalf("OpenShared(%s): %v", Adapter, err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		t.Fatalf("borrowed conn ping after Close: %v", err)
	}
}
