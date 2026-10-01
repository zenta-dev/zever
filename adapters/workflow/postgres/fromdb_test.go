package postgres

import (
	"path/filepath"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// TestNewFromDBBehavesIdentically builds a driver over an injected sqlite
// DB and exercises the Start/Query path from TestCreateRunToComplete.
func TestNewFromDBBehavesIdentically(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fromdb.db")

	conn, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	w, err := NewFromDB(conn, Options{Owner: "owner-fromdb"})
	if err != nil {
		t.Fatalf("NewFromDB failed: %v", err)
	}

	d, ok := w.(*driver)
	if !ok {
		t.Fatalf("NewFromDB returned %T, want *driver", w)
	}

	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "hello", "run-fromdb")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if id != workflow.RunID("run-fromdb") {
		t.Fatalf("RunID = %q, want %q", id, "run-fromdb")
	}

	var out string
	if err := d.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "hello" {
		t.Fatalf("state = %q, want %q", out, "hello")
	}

	// Borrowed connection: driver Close must not close the injected DB.
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("injected DB closed by driver Close: %v", err)
	}
}

// TestNewFromDBNilDB fails closed on a nil connection.
func TestNewFromDBNilDB(t *testing.T) {
	t.Parallel()

	if _, err := NewFromDB(nil, Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}
