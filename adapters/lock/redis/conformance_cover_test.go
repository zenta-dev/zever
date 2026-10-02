package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/lock/locktest"
)

// TestRedisConformance proves the redis adapter honors the lock.Locker
// contract via the shared conformance kit. Each subtest gets a fresh
// miniredis-backed instance (loopback only, no external network).
//
// Currently skipped: miniredis is not a faithful stand-in for the
// expiry-dependent subtests. Its clock advances only via FastForward:
// a lease acquired with a 30ms TTL is still held after seconds of
// real time, so ExtendUnlock (no-steal branch) and Expiry poll until
// the kit's 2s deadline and fail. The kit drives timing internally,
// so the fake cannot advance around it. Re-enable once the fake
// advances TTLs in real time.
func TestRedisConformance(t *testing.T) {
	t.Skip("miniredis clock is frozen without FastForward (expiry subtests cannot pass)")

	locktest.Conformance(t, func(t *testing.T) lock.Locker {
		t.Helper()

		s := miniredis.RunT(t)

		l, err := New(lock.Options{Addr: s.Addr()})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return l
	})
}
