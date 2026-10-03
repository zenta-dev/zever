package memory

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
)

func TestJanitorReapsExpired(t *testing.T) {
	// Not parallel: it mutates the package-level janitorInterval, which
	// parallel tests' janitor goroutines read.

	old := janitorInterval
	janitorInterval = 5 * time.Millisecond

	st, err := New(idempotency.Options{TTL: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	s := st.(*store) //nolint:forcetypeassert // New always returns *store

	if _, err := s.Begin(t.Context(), "k", idempotency.BeginOptions{Fingerprint: []byte("fp")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)

	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()

	for n != 0 {
		if time.Now().After(deadline) {
			t.Fatal("janitor did not reap expired record")
		}

		time.Sleep(5 * time.Millisecond)

		s.mu.Lock()
		n = len(s.entries)
		s.mu.Unlock()
	}

	// Restore only after the janitor has stopped.
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	janitorInterval = old
}
