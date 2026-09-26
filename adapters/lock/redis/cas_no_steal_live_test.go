package redis

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/lock"
)

func TestLive_staleExtendDoesNotStealSuccessor(t *testing.T) {
	s := miniredis.RunT(t)
	l, err := New(lock.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = l.Close(t.Context()) }()

	ctx := t.Context()

	stale, ok, err := l.TryAcquire(ctx, "job", 200*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want true, nil", ok, err)
	}

	s.FastForward(time.Second)

	next, ok, err := l.TryAcquire(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("successor TryAcquire = %v, %v, want true, nil", ok, err)
	}

	if err := stale.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("stale Extend = %v, want ErrNotHeld", err)
	}

	if err := stale.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Errorf("stale Unlock = %v, want ErrNotHeld", err)
	}

	if _, ok, err := l.TryAcquire(ctx, "job", time.Minute); err != nil || ok {
		t.Errorf("rival TryAcquire during successor lease = %v, %v, want false, nil", ok, err)
	}

	if err := next.Unlock(ctx); err != nil {
		t.Fatalf("successor Unlock: %v", err)
	}
}
