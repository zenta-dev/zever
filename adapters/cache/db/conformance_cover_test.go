package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/adapters/cache/db"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cache/cachetest"
	coredb "github.com/zenta-dev/zever/core/db"
)

// TestConformanceDB proves the DB-backed cache passes the cache kit
// against a file-backed sqlite database.
func TestConformanceDB(t *testing.T) {
	t.Parallel()

	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()

		c, err := db.New(db.Options{Options: coredb.Options{Path: filepath.Join(t.TempDir(), "cache.db")}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = c.Close(t.Context()) })

		return c
	})
}

// TestConformancePostgres proves the DB-backed cache passes the cache kit
// against a live postgres server. Skipped unless POSTGRES_DSN names one;
// never fake infra for conformance.
func TestConformancePostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres conformance against a live server")
	}

	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()

		c, err := db.New(db.Options{
			Options: coredb.Options{DSN: dsn},
			Table:   "cache_conformance",
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = c.Close(t.Context()) })

		return c
	})
}
