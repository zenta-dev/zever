package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/lock"
)

func TestExtend_afterTakeoverDoesNotStealLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newLocker(t, lock.Options{})

	stale, _, err := l.TryAcquire(ctx, "k", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	newHolder, ok, err := l.TryAcquire(ctx, "k", time.Minute)
	if err != nil || !ok {
		t.Fatalf("takeover acquire = (%v, %v), want (true, nil)", ok, err)
	}

	if err := stale.Extend(ctx, time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("stale extend = %v, want ErrNotHeld", err)
	}

	if _, ok, _ := l.TryAcquire(ctx, "k", time.Minute); ok {
		t.Fatal("stale extend stole the new holder lease")
	}

	if err := newHolder.Unlock(ctx); err != nil {
		t.Fatalf("new holder unlock: %v", err)
	}

	if err := stale.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("stale unlock after takeover = %v, want ErrNotHeld", err)
	}
}
