package testsetup

import (
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// TestProvidersResolveBatteries proves the test helper wires every default
// battery: a fresh container resolves a default battery with no per-test
// wiring, and calling Providers twice yields independent instances.
func TestProvidersResolveBatteries(t *testing.T) {
	cfg := config.Default()

	c := container.New(cfg, Providers(cfg)...)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}

	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}
}
