package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/queue/queuetest"
)

// TestRedisConformance proves the redis adapter honors the queue.Queue
// contract via the shared conformance kit. Each subtest gets a fresh
// miniredis-backed instance (loopback only, no external network) with
// the kit's short poll timeout so empty-queue cases stay fast. Tests
// here are sequential (no t.Parallel): the adapter shares process-wide
// script state with the other live tests in this package.
//
// The adapter stores headers as JSON objects; miniredis Lua re-encodes
// empty objects as arrays, which the adapter's tolerant wire decoder
// accepts. Sub-second BLPop waits run client-side so empty pops stay
// within the kit's short poll timeout instead of the server's 1s floor.
func TestRedisConformance(t *testing.T) {
	queuetest.Conformance(t, func(t *testing.T) queue.Queue {
		t.Helper()

		s := miniredis.RunT(t)

		q, err := New(queue.Options{Addr: s.Addr(), PollTimeout: queuetest.DefaultPollTimeout})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = q.Close() })

		return q
	})
}
