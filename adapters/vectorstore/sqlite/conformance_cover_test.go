package sqlite

import (
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/vectorstore/vectorstoretest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name so it cannot collide with other registrations in
// this binary; the kit factory below opens via New directly.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = vectorstore.Adapter("kit-vectorstore-sqlite")

	if err := vectorstore.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	s, err := vectorstore.Open(adapter, vectorstore.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if err := s.Upsert(t.Context(), vectorstore.Vector{ID: "wiring", Embedding: []float32{1, 0}}); err != nil {
		t.Errorf("Upsert() error = %v", err)
	}
}

// TestConformance runs the shared vectorstore kit against sqlite.
func TestConformance(t *testing.T) {
	t.Parallel()

	vectorstoretest.Conformance(t, func(t *testing.T) vectorstore.VectorStore {
		t.Helper()

		s, err := New(vectorstore.Options{DSN: ":memory:"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
