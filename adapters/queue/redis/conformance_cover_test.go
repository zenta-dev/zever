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
// Currently skipped: miniredis's Lua cjson is not faithful. claim.lua
// decodes the stored message and re-encodes it, and miniredis encodes
// an empty Lua table as a JSON array: stored `"headers":{}` comes back
// as `"headers":[]`, so decodeMessage fails with "cannot unmarshal JSON
// array into Go queue.Headers" on every Pop of a message with nil/empty
// headers (FIFO, Empty, LengthIsEmpty, Ack, NackDrop, Delayed,
// TopicsIsolated). Separately, miniredis floors BLPop timeouts below 1s
// up to 1s, which would make every empty-pop case take a second.
// Re-enable once the fake preserves empty JSON objects through scripts.
func TestRedisConformance(t *testing.T) {
	t.Skip("miniredis Lua cjson mangles empty headers ({} -> []) and floors BLPop below 1s")

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
