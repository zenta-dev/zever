package redis

import (
	"os"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/queue/queuetest"
)

// TestQueueConformanceLive runs the shared queue conformance kit against a
// real Redis server named by ZEVER_REDIS_ADDR (host:port). Skipped unless
// the variable is set, so local runs without docker stay green; the
// conformance-live CI workflow sets it from its redis service container.
// Miniredis coverage lives in conformance_cover_test.go; the live pass
// exercises the real Lua scripts miniredis only emulates.
func TestQueueConformanceLive(t *testing.T) {
	addr := os.Getenv("ZEVER_REDIS_ADDR")
	if addr == "" {
		t.Skip("set ZEVER_REDIS_ADDR to run queue conformance against a live server")
	}

	queuetest.Conformance(t, func(t *testing.T) queue.Queue {
		t.Helper()

		raw := goredis.NewClient(&goredis.Options{Addr: addr})
		t.Cleanup(func() { _ = raw.Close() })

		if err := raw.FlushDB(t.Context()).Err(); err != nil {
			t.Fatalf("FlushDB() error = %v", err)
		}

		q, err := New(queue.Options{Addr: addr, PollTimeout: queuetest.DefaultPollTimeout})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = q.Close() })

		return q
	})
}
