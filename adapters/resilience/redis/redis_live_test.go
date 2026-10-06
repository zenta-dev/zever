package redis_test

import (
	"context"
	"os"
	"testing"

	"github.com/zenta-dev/zever/adapters/resilience/redis"
	"github.com/zenta-dev/zever/core/resilience"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// TestLiveRedis exercises the adapter against a real Redis. Run with
// REDIS_ADDR=host:port, e.g. REDIS_ADDR=localhost:6379 go test -run TestLiveRedis ./...
func TestLiveRedis(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set")
	}

	m, err := redis.New(redis.Options{
		Options: resilience.Options{
			Redis: resilience.RedisOptions{
				Options: redisopt.Options{
					ConnectOptions: redisopt.ConnectOptions{Addr: addr},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	defer func() { _ = m.Close() }()

	g, err := m.Guard("live")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if g.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed", g.State())
	}
}
