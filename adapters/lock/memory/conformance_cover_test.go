package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/lock/locktest"
)

// TestMemoryConformance proves the memory adapter honors the lock.Locker
// contract via the shared conformance kit. Each subtest gets a fresh
// in-process instance (no network).
func TestMemoryConformance(t *testing.T) {
	locktest.Conformance(t, func(t *testing.T) lock.Locker {
		t.Helper()

		l, err := New(lock.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return l
	})
}
