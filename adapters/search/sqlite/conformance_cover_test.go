package sqlite

import (
	"testing"

	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/search/searchtest"
)

// TestOpenRegister proves Register wiring plus Open round-trip. It uses a
// test-only adapter name so it cannot collide with other registrations in
// this binary; the kit factory below opens via New directly.
func TestOpenRegister(t *testing.T) {
	t.Parallel()

	const adapter = search.Adapter("kit-search-sqlite")

	if err := search.Register(adapter, New); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	s, err := search.Open(adapter, search.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if err := s.Index(t.Context(), search.Document{ID: "wiring", Index: "kit", Content: "wiring probe"}); err != nil {
		t.Errorf("Index() error = %v", err)
	}
}

// TestConformance runs the shared search kit against sqlite.
func TestConformance(t *testing.T) {
	t.Parallel()

	searchtest.Conformance(t, func(t *testing.T) search.Search {
		t.Helper()

		s, err := New(search.Options{DSN: ":memory:"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
