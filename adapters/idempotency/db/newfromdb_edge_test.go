package db

import (
	"context"
	"path/filepath"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
)

// TestNewFromDB_nilConn checks the fail-closed nil guard.
func TestNewFromDB_nilConn(t *testing.T) {
	t.Parallel()

	_, err := NewFromDB(nil, Options{})
	if err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

// TestNewFromDB_borrowsConnection checks that a store built over a caller's
// connection does not own it: Close leaves the pool open.
func TestNewFromDB_borrowsConnection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "borrow.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	s, err := NewFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB: %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("NewFromDB type = %T, want *driver", s)
	}

	if d.owns {
		t.Fatal("NewFromDB marked borrowed conn as owned")
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("borrowed conn ping after Close: %v", err)
	}
}

// TestOpenFromDB_delegates checks that OpenFromDB mirrors NewFromDB.
func TestOpenFromDB_delegates(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "delegate.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	s, err := OpenFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("OpenFromDB: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
