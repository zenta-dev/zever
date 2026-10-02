package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cache/cachetest"
)

// TestRedisConformance proves the redis adapter honors the cache.Cache
// contract via the shared conformance kit. Each subtest gets a fresh
// miniredis-backed instance (loopback only, no external network).
// Tests here are sequential (no t.Parallel): New threads through the
// shared client singleton, so parallelism is forbidden like the other
// live tests in this package.
//
// Miniredis TTLs advance only via FastForward, so the factory wraps the
// adapter to implement the kit's FastForward seam: expiry polls advance
// the fake clock by the poll interval after each unsuccessful attempt.
func TestRedisConformance(t *testing.T) {
	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()

		s := miniredis.RunT(t)

		c, err := New(cache.Options{Addr: s.Addr()})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = c.Close(t.Context()) })

		return &fastForwardCache{Cache: c, s: s}
	})
}

// fastForwardCache wraps a cache.Cache with the kit's virtual-time seam,
// advancing the backing miniredis clock on every expiry poll.
type fastForwardCache struct {
	cache.Cache
	s *miniredis.Miniredis
}

// FastForward advances the fake clock backing the conformance instance.
func (c *fastForwardCache) FastForward(d time.Duration) { c.s.FastForward(d) }
