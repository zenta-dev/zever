package container

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestContainer_DoubleClose_NoError pins the idempotent shutdown contract:
// a second Close over the same container succeeds rather than erroring.
func TestContainer_DoubleClose_NoError(t *testing.T) {
	t.Parallel()

	registerTestAdapters()
	c := New(config.Default())
	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}

	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestContainer_ResolveNilContainer pins the nil-receiver guard: resolving a
// plugin on a nil container returns an error instead of panicking.
func TestContainer_ResolveNilContainer(t *testing.T) {
	t.Parallel()

	if _, err := Resolve[any](nil, "anything"); err == nil {
		t.Fatal("Resolve(nil, ...) = nil error, want error")
	}
}

// TestContainer_ResolveUnregistered pins the unknown-plugin error path.
func TestContainer_ResolveUnregistered(t *testing.T) {
	t.Parallel()

	c := New(config.Default())
	_, err := Resolve[any](c, "definitely-not-registered")
	if err == nil {
		t.Fatal("want error for unregistered plugin")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("err = %v, want not-registered error", err)
	}
}
