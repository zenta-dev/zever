package container

import (
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestNew_nilConfigResolvesDefaults pins New(nil) falling back to
// config.Default: the default cache adapter must resolve.
func TestNew_nilConfigResolvesDefaults(t *testing.T) {
	t.Parallel()

	c := New(nil)
	if c == nil {
		t.Fatal("New(nil) returned nil")
	}

	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	if _, err := c.Cache(); err != nil {
		t.Fatalf("New(nil) container failed to resolve Cache: %v", err)
	}
}

// TestNew_storesConfig pins that the passed config drives resolution:
// a bogus adapter fails, the real one succeeds on the same container.
func TestNew_storesConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Cache.Adapter = "bogus-adapter"

	c := New(cfg)
	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})
	if _, err := c.Cache(); err == nil {
		t.Fatal("expected error for bogus cache adapter, got nil")
	}

	cfg.Cache.Adapter = "memory"

	if _, err := c.Cache(); err != nil {
		t.Fatalf("expected success after fixing adapter, got: %v", err)
	}
}

// TestSealed_callable is a compile pin: the seal method must exist and run.
func TestSealed_callable(t *testing.T) {
	t.Parallel()

	(&Container{}).sealed()
}
