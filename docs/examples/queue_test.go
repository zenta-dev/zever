package examples_test

import (
	"context"
	"fmt"
	"time"

	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/queue"
)

// ExampleQueue_pushPopAck mirrors the queue doc sample: push a
// confirmation payload, pop it in the worker, ack on success, and report
// the remaining depth.
func Example_queuePushPopAck() {
	q, err := queuememory.New(queue.Options{
		VisibilityTimeout: 30 * time.Second,
		PollTimeout:       time.Second,
		Buffer:            100,
	})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer func() { _ = q.Close() }()

	ctx := context.Background()
	payload := queue.NewPayload([]byte(`{"booking_id":"b_123"}`))
	headers := queue.NewHeaders(map[string]string{"job_name": "SendConfirmation"})
	if err := q.Push(ctx, "default", payload, headers); err != nil {
		fmt.Println("push error")
		return
	}
	msg, err := q.Pop(ctx, "default")
	if err != nil {
		fmt.Println("pop error")
		return
	}
	fmt.Println(string(msg.Payload), msg.Headers["job_name"])

	if err := q.Ack(ctx, msg); err != nil {
		fmt.Println("ack error")
		return
	}
	fmt.Println("acked")

	n, err := q.Length(ctx, "default")
	if err != nil {
		fmt.Println("length error")
		return
	}
	fmt.Println("remaining", n)
	// Output:
	// {"booking_id":"b_123"} SendConfirmation
	// acked
	// remaining 0
}
