package postgres

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/search/searchtest"
)

// TestConformance runs the shared search kit against postgres via
// Register+Open. Skipped unless SEARCH_PG_DSN names a live server; unit
// coverage with fakes lives in postgres_test.go. Never fake infra for
// conformance.
func TestConformance(t *testing.T) {
	dsn := os.Getenv("SEARCH_PG_DSN")
	if dsn == "" {
		t.Skip("SEARCH_PG_DSN not set; skipping postgres conformance against a live server")
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
}
