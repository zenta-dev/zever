package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
)

// newInternalStore opens a store and returns it as *store so edge tests can
// inspect the lease table directly.
func newInternalStore(t *testing.T, maxEntries int) *store {
	t.Helper()

	st, err := New(idempotency.Options{MaxEntries: maxEntries})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	s, ok := st.(*store)
	if !ok {
		t.Fatalf("New type = %T, want *store", st)
	}

	return s
}

func TestEdge_EntryExpiredBoundary(t *testing.T) {
	t.Parallel()

	now := time.Now()
	e := &entry{expires: now}

	if e.expired(now) {
		t.Error("expired(exact deadline) = true, want false")
	}

	if !e.expired(now.Add(time.Nanosecond)) {
		t.Error("expired(after deadline) = false, want true")
	}
}

func TestEdge_SweepRemovesOnlyExpired(t *testing.T) {
	t.Parallel()

	s := newInternalStore(t, 10)

	s.mu.Lock()
	s.entries["live"] = &entry{expires: time.Now().Add(time.Hour)}
	s.entries["dead"] = &entry{expires: time.Now().Add(-time.Hour)}
	s.mu.Unlock()

	s.sweep()

	s.mu.Lock()
	_, live := s.entries["live"]
	_, dead := s.entries["dead"]
	s.mu.Unlock()

	if !live {
		t.Error("sweep removed a live record")
	}

	if dead {
		t.Error("sweep kept an expired record")
	}
}

func TestEdge_EvictLockedVictimSoonestExpiry(t *testing.T) {
	t.Parallel()

	s := newInternalStore(t, 2)
	now := time.Now()

	s.mu.Lock()
	s.entries["a"] = &entry{expires: now.Add(2 * time.Hour)}
	s.entries["b"] = &entry{expires: now.Add(time.Hour)}
	s.evictLocked(now)
	_, hasA := s.entries["a"]
	_, hasB := s.entries["b"]
	s.mu.Unlock()

	if !hasA {
		t.Error("evictLocked removed the later-expiring record")
	}

	if hasB {
		t.Error("evictLocked kept the soonest-expiring record")
	}
}

func TestEdge_EvictLockedEmptyNoop(t *testing.T) {
	t.Parallel()

	s := newInternalStore(t, 10)

	s.mu.Lock()
	s.maxEntries = 0
	s.evictLocked(time.Now())
	n := len(s.entries)
	s.mu.Unlock()

	if n != 0 {
		t.Fatalf("entries = %d, want 0", n)
	}
}

func TestEdge_CompleteExpiredRecordReclaimed(t *testing.T) {
	t.Parallel()

	s := newInternalStore(t, 10)
	ctx := t.Context()

	s.mu.Lock()
	s.entries[s.prefix+"key"] = &entry{fp: []byte("old"), expires: time.Now().Add(-time.Hour)}
	s.mu.Unlock()

	if err := s.Complete(ctx, "key", []byte("new"), []byte("res")); err != nil {
		t.Fatalf("Complete over expired record: %v", err)
	}

	out, err := s.Begin(ctx, "key", idempotency.BeginOptions{Fingerprint: []byte("new")})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "res" {
		t.Fatalf("replay = %+v, want {true res}", out)
	}
}
