package queue_test

import (
	"context"

	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/queue/memory"
)

// ExampleOpen opens the in-memory queue and pushes a message.
func ExampleOpen() {
	_ = queue.Register(queue.Memory, memory.New)

	q, err := queue.Open(queue.Memory, queue.Options{})
	if err != nil {
		return
	}
	defer q.Close()

	ctx := context.Background()
	_ = q.Push(ctx, "jobs", queue.NewPayload([]byte("hi")), queue.NewHeaders(map[string]string{"k": "v"}))
}
