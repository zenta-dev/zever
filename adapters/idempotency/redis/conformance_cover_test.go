package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/idempotency/idempotencytest"
)

// TestRedisConformance proves the redis adapter honors the
// idempotency.Store contract via the shared conformance kit. Each
// subtest gets a fresh miniredis-backed instance (loopback only, no
// external network).
func TestRedisConformance(t *testing.T) {
	idempotencytest.Conformance(t, func(t *testing.T) idempotency.Store {
		t.Helper()

		s := miniredis.RunT(t)

		st, err := New(idempotency.Options{Redis: idempotency.RedisOptions{Addr: s.Addr()}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = st.Close() })

		return st
	})
}
