package container

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

// TestNew_nilConfigResolvesDefaults pins New(nil) falling back to
// config.Default: the container builds, and a wired cache provider
// resolves.
func TestNew_nilConfigResolvesDefaults(t *testing.T) {
	t.Parallel()

	c := New(nil, Provide(cache.DefaultRef(), newFakeCache))
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

// TestNew_unwiredAccessorsFailNotWired pins the provider-only contract: a
// container with no providers resolves nothing, and every accessor names
// its missing provider instead of falling back to any config path.
func TestNew_unwiredAccessorsFailNotWired(t *testing.T) {
	t.Parallel()

	c := New(nil)
	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	if _, err := c.Cache(); !isNotWiredFor(err, "cache") {
		t.Fatalf("Cache() = %v, want NotWiredError for cache", err)
	}

	var nw NotWiredError
	if _, err := c.DB(); !errors.As(err, &nw) || !errors.Is(err, ErrNotWired) {
		t.Fatalf("DB() = %v, want NotWiredError wrapping ErrNotWired", err)
	}
}

// TestSealed_callable is a compile pin: the seal method must exist and run.
func TestSealed_callable(t *testing.T) {
	t.Parallel()

	(&Container{}).sealed()
}
