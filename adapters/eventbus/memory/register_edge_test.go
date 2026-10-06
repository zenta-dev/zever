package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestRegister proves Register wires the memory adapter into the eventbus
// registry, so Open resolves it to a live bus.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	b, err := eventbus.Open(eventbus.Memory, eventbus.Options{})
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", eventbus.Memory, err)
	}

	if got := b.Name(); got != "memory" {
		t.Fatalf("Name() = %q, want memory", got)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
