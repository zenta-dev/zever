package memory

import (
	"testing"
	"time"
)

func TestRevokePastExpiryIsNoOp(t *testing.T) {
	t.Parallel()

	si, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	s, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}

	if err := s.Revoke(t.Context(), "past-jti", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("Revoke(past) error = %v, want nil", err)
	}
	if revoked, err := s.IsRevoked(t.Context(), "past-jti"); err != nil || revoked {
		t.Fatalf("IsRevoked(past) = %v, %v, want false, nil", revoked, err)
	}

	s.mu.Lock()
	n := len(s.revoked)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("revoked len = %d, want 0 (already-lapsed entry must not be stored)", n)
	}
}
