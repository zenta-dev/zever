package lock_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/adapters/lock/memory"
	"github.com/zenta-dev/zever/core/lock"
)

// ExampleOpen acquires and releases a memory lease.
func ExampleOpen() {
	if err := lock.Register(lock.Memory, memory.New); err != nil && !errors.Is(err, lock.ErrDuplicate) {
		return
	}

	l, err := lock.Open(lock.Memory, lock.Options{})
	if err != nil {
		return
	}

	ctx := context.Background()

	h, ok, err := l.TryAcquire(ctx, "example-key", 0)
	if err != nil || !ok {
		return
	}

	fmt.Println(ok)

	if err := h.Unlock(ctx); err != nil {
		return
	}

	_ = l.Close(ctx)

	// Output: true
}
