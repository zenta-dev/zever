package db

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/search/searchtest"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// TestConformance runs the shared search kit against the migrated adapter.
// The db leg runs on :memory: with no infra; the postgres leg points the
// same "db" adapter at a live server and runs only when POSTGRES_DSN
// (fallback SEARCH_PG_DSN) names one.
func TestConformance(t *testing.T) {
	t.Run("db", func(t *testing.T) {
		searchtest.Conformance(t, func(t *testing.T) search.Search {
			t.Helper()

			s, err := New(Options{})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}

			t.Cleanup(func() { _ = s.Close() })

			return s
		})
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			dsn = os.Getenv("SEARCH_PG_DSN")
		}

		if dsn == "" {
			t.Skip("POSTGRES_DSN/SEARCH_PG_DSN not set; skipping postgres conformance against a live server")
		}

		searchtest.Conformance(t, func(t *testing.T) search.Search {
			t.Helper()

			poolOpts := dbconn.SplitDSN(dsn)
			s, err := New(Options{Options: poolOpts})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}

			t.Cleanup(func() { _ = s.Close() })

			return s
		})
	})
}
