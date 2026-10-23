package queue_test

import (
	"context"

	"github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/queue"
)

// Example opens the in-memory queue and pushes a message.
func Example() {
	q, err := memory.New(queue.Options{})
	if err != nil {
		return
	}
	defer q.Close()

	ctx := context.Background()
	_ = q.Push(ctx, "jobs", queue.NewPayload([]byte("hi")), queue.NewHeaders(map[string]string{"k": "v"}))
}
