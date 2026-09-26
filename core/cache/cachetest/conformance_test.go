package cachetest_test

import (
	"testing"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cache/cachetest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()

		c, err := cachememory.New(cache.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = c.Close(t.Context()) })

		return c
	})
}
