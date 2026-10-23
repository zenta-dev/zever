package ratelimit_test

import (
	"context"

	"github.com/zenta-dev/zever/adapters/ratelimit/memory"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// Example opens the in-memory limiter and allows one token.
func Example() {
	lim, err := memory.New(ratelimit.Options{Rate: 10, Burst: 20})
	if err != nil {
		return
	}
	defer lim.Close()

	_, _ = lim.Allow(context.Background(), "user-1", 1)
}
