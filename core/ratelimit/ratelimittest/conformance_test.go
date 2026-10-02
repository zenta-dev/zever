package ratelimittest_test

import (
	"testing"

	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/ratelimit/ratelimittest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	ratelimittest.Conformance(t, func(t *testing.T) ratelimit.Limiter {
		t.Helper()

		l, err := ratelimitmemory.New(ratelimit.Options{Rate: 10, Burst: 3})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close() })

		return l
	})
}
