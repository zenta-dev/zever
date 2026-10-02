package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/eventbus/eventbustest"
)

// TestRedisConformance proves the redis adapter honors the
// eventbus.EventBus contract via the shared conformance kit. Each
// subtest gets a fresh miniredis-backed instance (loopback only, no
// external network).
func TestRedisConformance(t *testing.T) {
	eventbustest.Conformance(t, func(t *testing.T) eventbus.EventBus {
		t.Helper()

		s := miniredis.RunT(t)

		b, err := New(eventbus.Options{Redis: eventbus.RedisOptions{Addr: s.Addr()}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
