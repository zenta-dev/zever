package eventbustest_test

import (
	"testing"

	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/eventbus/eventbustest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	eventbustest.Conformance(t, func(t *testing.T) eventbus.EventBus {
		t.Helper()

		b, err := eventbusmemory.New(eventbus.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
