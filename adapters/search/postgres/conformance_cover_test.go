package postgres

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/search/searchtest"
)

// TestConformance runs the shared search kit against the migrated adapter.
// The sqlite leg runs on :memory: with no infra; the postgres leg runs only
// when POSTGRES_DSN (fallback SEARCH_PG_DSN) names a live server.
func TestConformance(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		searchtest.Conformance(t, func(t *testing.T) search.Search {
			t.Helper()

			Register()

			s, err := search.Open(search.SQLite, search.Options{})
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

			Register()

			s, err := search.Open(search.Postgres, search.Options{DSN: dsn})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}

			t.Cleanup(func() { _ = s.Close() })

			return s
		})
	})
}
