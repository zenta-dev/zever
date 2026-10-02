package redis_test

import (
	"testing"

	sredis "github.com/zenta-dev/zever/adapters/session/redis"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/session/sessiontest"
)

// TestConformanceRedis proves the redis store passes the session kit
// against an in-process miniredis.
func TestConformanceRedis(t *testing.T) {
	t.Parallel()

	sessiontest.Conformance(t, func(t *testing.T) session.Store {
		t.Helper()

		srv := testServer(t)

		s, err := sredis.New(optionsFor(t, srv))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}
