package redis

import (
	"os"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cache/cachetest"
)

// TestRedisConformanceLive runs the shared cache conformance kit against a
// real Redis server named by ZEVER_REDIS_ADDR (host:port). Skipped unless
// the variable is set, so local runs without docker stay green; the
// conformance-live CI workflow sets it from its redis service container.
// Never fake infra for conformance: miniredis coverage lives in
// conformance_cover_test.go.
//
// Tests here are sequential (no t.Parallel): New threads through the shared
// client singleton, like the other live tests in this package. Each kit
// subtest gets a freshly flushed logical database so fixed kit keys stay
// isolated.
func TestRedisConformanceLive(t *testing.T) {
	addr := os.Getenv("ZEVER_REDIS_ADDR")
	if addr == "" {
		t.Skip("set ZEVER_REDIS_ADDR to run redis conformance against a live server")
	}

	cachetest.Conformance(t, func(t *testing.T) cache.Cache {
		t.Helper()

		raw := goredis.NewClient(&goredis.Options{Addr: addr})
		t.Cleanup(func() { _ = raw.Close() })

		if err := raw.FlushDB(t.Context()).Err(); err != nil {
			t.Fatalf("FlushDB() error = %v", err)
		}

		c, err := New(cache.Options{Addr: addr})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = c.Close(t.Context()) })

		return c
	})
}
