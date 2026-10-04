package examples_test

import (
	"errors"
	"testing"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/shared/codec"
)

// TestTypedCacheMiss pins the missing-key boundary: a typed Get on an unset
// key returns ErrNotFound rather than a zero value with no error.
func TestTypedCacheMiss(t *testing.T) {
	t.Parallel()

	backend, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close(t.Context()) })

	typed := cache.NewTyped[string, spaceCard](backend, codec.JSONCodec[spaceCard]{})
	if _, err := typed.Get(t.Context(), "missing"); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

// TestCacheDoubleClose pins the idempotent Close contract for the memory
// backend used throughout these examples.
func TestCacheDoubleClose(t *testing.T) {
	t.Parallel()

	backend, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := backend.Close(t.Context()); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := backend.Close(t.Context()); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestPluginResolveUnknown pins the unknown-plugin error path.
func TestPluginResolveUnknown(t *testing.T) {
	t.Parallel()

	c := container.New(nil)
	if _, err := container.Resolve[greeter](c, "docs-example-not-registered"); err == nil {
		t.Fatal("Resolve unknown = nil error, want error")
	}
}
