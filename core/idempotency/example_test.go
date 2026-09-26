package idempotency_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/adapters/idempotency/memory"
	"github.com/zenta-dev/zever/core/idempotency"
)

// ExampleOpen opens the memory store and replays a completed key.
func ExampleOpen() {
	if err := idempotency.Register(idempotency.Memory, memory.New); err != nil {
		return
	}

	store, err := idempotency.Open(idempotency.Memory, idempotency.Options{})
	if err != nil {
		return
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	first, err := store.Begin(ctx, "order-1", idempotency.BeginOptions{})
	if err != nil || first.Replay {
		return
	}

	if completeErr := store.Complete(ctx, "order-1", nil, []byte("ok")); completeErr != nil {
		return
	}

	replay, err := store.Begin(ctx, "order-1", idempotency.BeginOptions{})
	if err != nil || !replay.Replay {
		return
	}

	fmt.Println(string(replay.Result))
	// Output: ok
}
