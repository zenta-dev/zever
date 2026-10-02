package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/ratelimit/ratelimittest"
)

// TestMemoryConformance proves the memory adapter honors the
// ratelimit.Limiter contract via the shared conformance kit. Each
// subtest gets a fresh in-process instance (no network) with a small
// burst so the allow-then-deny sequence stays deterministic.
func TestMemoryConformance(t *testing.T) {
	ratelimittest.Conformance(t, func(t *testing.T) ratelimit.Limiter {
		t.Helper()

		l, err := New(ratelimit.Options{Rate: 10, Burst: 3})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close() })

		return l
	})
}
