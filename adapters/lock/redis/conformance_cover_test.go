package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/lock/locktest"
)

// TestRedisConformance proves the redis adapter honors the lock.Locker
// contract via the shared conformance kit. Each subtest gets a fresh
// miniredis-backed instance (loopback only, no external network).
//
// Miniredis TTLs advance only via FastForward, so the factory wraps the
// adapter to implement the kit's FastForward seam: expiry polls advance
// the fake clock by the poll interval after each unsuccessful attempt.
func TestRedisConformance(t *testing.T) {
	locktest.Conformance(t, func(t *testing.T) lock.Locker {
		t.Helper()

		s := miniredis.RunT(t)

		l, err := New(lock.Options{Addr: s.Addr()})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = l.Close(t.Context()) })

		return &fastForwardLocker{Locker: l, s: s}
	})
}

// fastForwardLocker wraps a lock.Locker with the kit's virtual-time seam,
// advancing the backing miniredis clock on every expiry poll.
type fastForwardLocker struct {
	lock.Locker
	s *miniredis.Miniredis
}

// FastForward advances the fake clock backing the conformance instance.
func (l *fastForwardLocker) FastForward(d time.Duration) { l.s.FastForward(d) }
