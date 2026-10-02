package idempotencytest_test

import (
	"testing"

	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/idempotency/idempotencytest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	idempotencytest.Conformance(t, func(t *testing.T) idempotency.Store {
		t.Helper()

		s, err := idempotencymemory.New(idempotency.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
