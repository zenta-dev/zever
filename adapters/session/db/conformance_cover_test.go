package db_test

import (
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/adapters/session/db"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/session/sessiontest"
)

// TestConformanceDB proves the DB-backed store passes the session kit
// against a file-backed sqlite database.
func TestConformanceDB(t *testing.T) {
	t.Parallel()

	sessiontest.Conformance(t, func(t *testing.T) session.Store {
		t.Helper()

		s, err := db.New(db.Options{Options: coredb.Options{Path: filepath.Join(t.TempDir(), "sessions.db")}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
