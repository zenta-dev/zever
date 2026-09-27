package ratelimit_test

import (
	"context"

	"github.com/zenta-dev/zever/adapters/ratelimit/memory"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// ExampleOpen opens the in-memory limiter and allows one token.
func ExampleOpen() {
	_ = ratelimit.Register(ratelimit.Memory, memory.New)

	lim, err := ratelimit.Open(ratelimit.Memory, ratelimit.Options{Rate: 10, Burst: 20})
	if err != nil {
		return
	}
	defer lim.Close()

	_, _ = lim.Allow(context.Background(), "user-1", 1)
}
