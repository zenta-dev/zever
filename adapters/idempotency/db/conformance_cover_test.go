package db

import (
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/idempotency/idempotencytest"
)

// TestDBConformance proves the db adapter honors the idempotency.Store
// contract via the shared conformance kit. Each subtest gets a fresh
// sqlite :memory: database (no network, no files).
func TestDBConformance(t *testing.T) {
	idempotencytest.Conformance(t, func(t *testing.T) idempotency.Store {
		t.Helper()

		s, err := New(Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
