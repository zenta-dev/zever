package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/eventbus/eventbustest"
)

// TestMemoryConformance proves the memory adapter honors the
// eventbus.EventBus contract via the shared conformance kit. Each
// subtest gets a fresh in-process instance (no network).
func TestMemoryConformance(t *testing.T) {
	eventbustest.Conformance(t, func(t *testing.T) eventbus.EventBus {
		t.Helper()

		b, err := New(eventbus.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
