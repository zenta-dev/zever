package memory

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth/jwt/revocation"
	"github.com/zenta-dev/zever/auth/jwt/revocation/revocationtest"
)

func newTestStore(t *testing.T) revocation.Store {
	t.Helper()

	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return s
}

func TestStoreContract(t *testing.T) {
	t.Parallel()
	revocationtest.Run(t, newTestStore)
}

func TestNew_DefaultMaxEntries(t *testing.T) {
	t.Parallel()

	s, err := New(Options{MaxEntries: -1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", s)
	}
	if st.max != DefaultMaxEntries {
		t.Fatalf("max = %d, want %d", st.max, DefaultMaxEntries)
	}
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()

	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}
}

func TestRevoke_AfterClose(t *testing.T) {
	t.Parallel()

	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Revoke(context.Background(), "jti", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("Revoke() after Close succeeded, want error")
	}
}

func TestIsRevoked_AfterClose(t *testing.T) {
	t.Parallel()

	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := s.IsRevoked(context.Background(), "jti"); err == nil {
		t.Fatal("IsRevoked() after Close succeeded, want error")
	}
}

func TestCapacity_EvictsOldest(t *testing.T) {
	t.Parallel()

	si, err := New(Options{MaxEntries: 4})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	s, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}

	future := time.Now().Add(time.Hour)
	if err := s.Revoke(context.Background(), "oldest", future); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Revoke(context.Background(), "filler-"+string(rune('a'+i)), future); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
	}

	if err := s.Revoke(context.Background(), "fresh", future); err != nil {
		t.Fatalf("Revoke() at capacity error = %v", err)
	}

	if len(s.revoked) != 4 {
		t.Fatalf("revoked len = %d, want 4 (evicted oldest)", len(s.revoked))
	}
	if _, ok := s.revoked["oldest"]; ok {
		t.Error("oldest entry survived capacity eviction")
	}
	if revoked, err := s.IsRevoked(context.Background(), "fresh"); err != nil || !revoked {
		t.Errorf("IsRevoked(fresh) = %v, %v, want true, nil", revoked, err)
	}
}

func TestPruneExpiredDirect(t *testing.T) {
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

	past := time.Now().Add(-time.Minute)
	s.revoked["old"] = past
	s.revoked["live"] = time.Now().Add(time.Hour)

	s.pruneOnce(time.Now())

	if _, ok := s.revoked["old"]; ok {
		t.Error("pruneOnce kept expired entry")
	}
	if _, ok := s.revoked["live"]; !ok {
		t.Error("pruneOnce dropped live entry")
	}
}

func TestEvictOldestCompacts(t *testing.T) {
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

	// 2000 stale order entries, none in the map: the loop runs to the end,
	// then the compact branch (head>1024 && head>len/2) fires.
	for i := 0; i < 2000; i++ {
		s.order = append(s.order, "stale")
	}
	s.evictOldestLocked()

	if s.head != 0 {
		t.Errorf("head = %d, want 0 after compact", s.head)
	}
	if len(s.order) != 0 {
		t.Errorf("order len = %d, want 0 after compact", len(s.order))
	}
}

func TestPrunerTickFiresPruneOnce(t *testing.T) {
	// Not parallel: swaps the package-level newTicker seam.

	fired := make(chan time.Time, 1)
	old := newTicker
	newTicker = func(time.Duration) *time.Ticker { return &time.Ticker{C: fired} }
	defer func() { newTicker = old }()

	si, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	s, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}

	s.mu.Lock()
	s.revoked["old-jti"] = time.Now().Add(-time.Minute)
	s.revoked["live"] = time.Now().Add(time.Hour)
	s.mu.Unlock()

	fired <- time.Now()

	deadline := time.After(time.Second)
	var live bool
	for {
		s.mu.Lock()
		_, gone := s.revoked["old-jti"]
		_, live = s.revoked["live"]
		s.mu.Unlock()
		if !gone {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("pruneOnce did not remove expired entry within 1s")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if !live {
		t.Error("pruneOnce dropped live entry")
	}
}
