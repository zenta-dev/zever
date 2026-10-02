package pgvector

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/vectorstore/vectorstoretest"
)

// TestConformance runs the shared vectorstore kit against pgvector via
// Register+Open. Skipped unless PGVECTOR_TEST_DSN names a live pgvector
// server; unit coverage with fakes lives in pgvector_test.go. Never fake
// infra for conformance.
func TestConformance(t *testing.T) {
	dsn := os.Getenv("PGVECTOR_TEST_DSN")
	if dsn == "" {
		t.Skip("PGVECTOR_TEST_DSN unset; skipping pgvector conformance against a live server")
	}

	vectorstoretest.Conformance(t, func(t *testing.T) vectorstore.VectorStore {
		t.Helper()

		Register()

		s, err := vectorstore.Open(vectorstore.PGVector, vectorstore.Options{DSN: dsn, Dimension: 3})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
