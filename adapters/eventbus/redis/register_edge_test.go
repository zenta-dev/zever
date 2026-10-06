package redis

import (
	"testing"

	"github.com/zenta-dev/zever/core/eventbus"
)

// TestRegister proves Register wires the redis adapter into the eventbus
// registry, so Open resolves it to a live bus over a broker.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	b, err := eventbus.Open(eventbus.Redis, testOptions(t))
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", eventbus.Redis, err)
	}

	if got := b.Name(); got != "redis" {
		t.Fatalf("Name() = %q, want redis", got)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
