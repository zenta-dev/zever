package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
)

func TestSweepReapsExpired(t *testing.T) {
	t.Parallel()

	st, err := New(idempotency.Options{TTL: time.Minute})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	s := st.(*store) //nolint:forcetypeassert // New always returns *store
	cur := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	setClock(s, func() time.Time { return cur })

	if _, err := s.Begin(t.Context(), "k", idempotency.BeginOptions{Fingerprint: []byte("fp")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	s.sweep()

	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()

	if n != 1 {
		t.Fatalf("sweep before expiry left %d entries, want 1", n)
	}

	cur = cur.Add(2 * time.Minute)
	s.sweep()

	s.mu.Lock()
	n = len(s.entries)
	s.mu.Unlock()

	if n != 0 {
		t.Fatalf("sweep after expiry left %d entries, want 0", n)
	}
}

// setClock swaps the store clock under its lock.
func setClock(s *store, now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func TestExpiryReclaim(t *testing.T) {
	t.Parallel()

	st, err := New(idempotency.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	s := st.(*store) //nolint:forcetypeassert // New always returns *store
	ctx := t.Context()
	ttl := time.Minute
	cur := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	setClock(s, func() time.Time { return cur })

	if _, err = s.Begin(ctx, "key-expire", idempotency.BeginOptions{TTL: ttl}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Live record: a second claim reports in-progress, not a fresh claim.
	if out, beginErr := s.Begin(ctx, "key-expire", idempotency.BeginOptions{TTL: ttl}); beginErr == nil && !out.Replay {
		t.Fatal("re-Begin before expiry claimed fresh")
	}

	cur = cur.Add(ttl + time.Second)

	out, err := s.Begin(ctx, "key-expire", idempotency.BeginOptions{TTL: ttl})
	if err != nil || out.Replay {
		t.Fatalf("re-Begin after expiry = (%+v, %v), want fresh claim", out, err)
	}
}
