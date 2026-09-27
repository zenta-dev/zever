package queuetest_test

import (
	"testing"
	"time"

	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/queue/queuetest"
)

// TestConformanceMemory proves the kit passes against the in-memory adapter.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	queuetest.Conformance(t, func(t *testing.T) queue.Queue {
		t.Helper()

		q, err := queuememory.New(queue.Options{
			Buffer:            128,
			PollTimeout:       20 * time.Millisecond,
			VisibilityTimeout: time.Second,
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = q.Close() })

		return q
	})
}
