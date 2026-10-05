package testsetup

import (
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// TestRegisterDefaultsResolvesBatteries proves the registration helper makes
// config.Default() resolvable: after RegisterDefaults, a fresh container
// opens a default battery (cache) without any per-test wiring, and the
// helper is idempotent.
func TestRegisterDefaultsResolvesBatteries(t *testing.T) {
	RegisterDefaults()
	RegisterDefaults()

	c := container.New(config.Default())
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}

	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}
}
