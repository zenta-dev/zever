package locktest_test

import (
	"testing"

	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/lock/locktest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	locktest.Conformance(t, func(t *testing.T) lock.Locker {
		t.Helper()

		l, err := lockmemory.New(lock.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return l
	})
}
