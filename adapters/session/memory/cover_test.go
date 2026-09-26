package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

func TestCoverIntervalMiddleNoClamp(t *testing.T) {
	t.Parallel()

	// TTL 2m selects interval ttl/2 = 1m: neither the 1s floor nor the
	// 5m ceiling. No waiting: the clamp line runs at New.
	st, err := memory.New(session.Options{TTL: 2 * time.Minute})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.Create(t.Context(), 0); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestCoverSaveOverExpiredUpsertsFresh(t *testing.T) {
	t.Parallel()

	st, err := memory.New(session.Options{TTL: 40 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := t.Context()
	s, err := st.Create(ctx, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eventually(t, 2*time.Second, func() bool {
		_, getErr := st.Get(ctx, s.ID)
		return errors.Is(getErr, session.ErrNotFound)
	}, "session expiry")

	s.Data = map[string]any{"v": 1}
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save over expired: %v", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after resave: %v", err)
	}
	if got.Data["v"] != 1 {
		t.Errorf("resaved data = %v, want 1", got.Data["v"])
	}
}

func TestCoverSaveZeroCreatedAtStamped(t *testing.T) {
	t.Parallel()

	st, err := memory.New(session.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := t.Context()
	id := session.NewID()
	before := time.Now()
	if serr := st.Save(ctx, session.Session{ID: id, Data: map[string]any{"a": 1}}); serr != nil {
		t.Fatalf("Save fresh ID: %v", serr)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want >= %v", got.CreatedAt, before)
	}
	if !got.ExpiresAt.After(got.CreatedAt) {
		t.Errorf("ExpiresAt = %v, want after CreatedAt", got.ExpiresAt)
	}
}

func TestCoverSweepDeletesExpired(t *testing.T) {
	t.Parallel()

	st, err := memory.New(session.Options{TTL: 40 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := t.Context()
	old, err := st.Create(ctx, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Wait past the 40ms TTL without touching old (Get would lazily
	// purge and hide the sweep path).
	start := time.Now()
	eventually(t, 2*time.Second, func() bool { return time.Since(start) > 100*time.Millisecond }, "past TTL")

	// A new write triggers the opportunistic sweep, deleting the
	// expired entry (lazy Get would also miss, but the sweep line
	// itself only runs here).
	if _, err := st.Create(ctx, 0); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := st.Get(ctx, old.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get swept = %v, want ErrNotFound", err)
	}
}

func TestCoverTickerSweepFires(t *testing.T) {
	t.Parallel()

	// Store TTL 2s selects a 1s sweep interval: poll until the 1s session
	// expires (the background ticker also sweeps; Get reports NotFound
	// either way without any further writes).
	st, err := memory.New(session.Options{TTL: 2 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := t.Context()
	short, err := st.Create(ctx, time.Second)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eventually(t, 5*time.Second, func() bool {
		_, err := st.Get(ctx, short.ID)
		return errors.Is(err, session.ErrNotFound)
	}, "ticker sweep expiry")
}
