package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/ratelimit/ratelimittest"
)

// TestRedisConformance proves the redis adapter honors the
// ratelimit.Limiter contract via the shared conformance kit. Each
// subtest gets a fresh miniredis-backed instance (loopback only, no
// external network) with a small burst so the allow-then-deny
// sequence stays deterministic.
func TestRedisConformance(t *testing.T) {
	ratelimittest.Conformance(t, func(t *testing.T) ratelimit.Limiter {
		t.Helper()

		s := miniredis.RunT(t)

		l, err := New(ratelimit.Options{Rate: 10, Burst: 3, Redis: ratelimit.RedisOptions{Addr: s.Addr()}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close() })

		return l
	})
}
