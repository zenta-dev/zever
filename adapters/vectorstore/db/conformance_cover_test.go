package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/vectorstore/vectorstoretest"
)

// TestConformance runs the shared vectorstore kit against the migrated adapter.
// The db leg runs on a per-store sqlite temp file with no infra; the pgvector leg points the
// same "db" adapter at a live server and runs only when POSTGRES_DSN (fallback
// PGVECTOR_TEST_DSN) names one. Never fake infra for conformance.
func TestConformance(t *testing.T) {
	t.Run("db", func(t *testing.T) {
		vectorstoretest.Conformance(t, func(t *testing.T) vectorstore.VectorStore {
			t.Helper()

			Register()

			// A per-store sqlite file, not the default ":memory:": the
			// sqlite adapter opens in-memory databases with cache=shared,
			// so every ":memory:" connection in this test binary sees the
			// same tables, and rows upserted by parallel tests (e.g.
			// TestNewFromDB's "borrowed") leak into the kit's queries.
			dsn := filepath.Join(t.TempDir(), "conformance.db")

			s, err := vectorstore.Open(vectorstore.DB, vectorstore.Options{DSN: dsn})
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
			dsn = os.Getenv("PGVECTOR_TEST_DSN")
		}

		if dsn == "" {
			t.Skip("POSTGRES_DSN/PGVECTOR_TEST_DSN not set; skipping postgres conformance against a live server")
		}

		vectorstoretest.Conformance(t, func(t *testing.T) vectorstore.VectorStore {
			t.Helper()

			Register()

			s, err := vectorstore.Open(vectorstore.DB, vectorstore.Options{DSN: dsn, Dimension: 3})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}

			t.Cleanup(func() { _ = s.Close() })

			return s
		})
	})
}
