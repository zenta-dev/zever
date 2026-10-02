package sessiontest_test

import (
	"testing"

	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/session/sessiontest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	sessiontest.Conformance(t, func(t *testing.T) session.Store {
		t.Helper()

		s, err := sessionmemory.New(session.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
