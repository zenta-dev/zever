package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/idempotency/idempotencytest"
)

// TestMemoryConformance proves the memory adapter honors the
// idempotency.Store contract via the shared conformance kit. Each
// subtest gets a fresh in-process instance (no network).
func TestMemoryConformance(t *testing.T) {
	idempotencytest.Conformance(t, func(t *testing.T) idempotency.Store {
		t.Helper()

		s, err := New(idempotency.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
