package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
)

// TestRegister proves Register wires the memory adapter into the idempotency
// registry, so Open resolves it to a live store.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	s, err := idempotency.Open(idempotency.Memory, idempotency.Options{})
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", idempotency.Memory, err)
	}

	if s == nil {
		t.Fatal("Open returned nil store")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestEdge_EvictLockedReapsExpired checks that a full table reaps an expired
// record before evicting a live one.
func TestEdge_EvictLockedReapsExpired(t *testing.T) {
	t.Parallel()

	s := newInternalStore(t, 2)
	now := time.Now()

	s.mu.Lock()
	s.entries["live"] = &entry{expires: now.Add(time.Hour)}
	s.entries["dead"] = &entry{expires: now.Add(-time.Hour)}
	s.evictLocked(now)
	_, live := s.entries["live"]
	_, dead := s.entries["dead"]
	s.mu.Unlock()

	if !live {
		t.Error("evictLocked removed a live record instead of the expired one")
	}

	if dead {
		t.Error("evictLocked kept the expired record")
	}
}
