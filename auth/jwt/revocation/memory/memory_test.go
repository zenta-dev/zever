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

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(5 * time.Millisecond)
	}
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

// TestCapacity_EvictsSoonestToExpire is the regression test for the
// eviction-policy bug: at capacity, the store must evict the entry closest
// to its natural expiry, not the oldest-inserted one. It inserts entries
// with a deliberate mismatch between insertion order and expiry order (the
// first-inserted entry has the *furthest* expiry, a middle entry has the
// *soonest*), forces an eviction, and asserts the soonest-to-expire entry
// is the one that's gone while the oldest-inserted (but far-from-expiry)
// entry survives.
func TestCapacity_EvictsSoonestToExpire(t *testing.T) {
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

	now := time.Now()

	// Inserted first, but far from expiry: under the old (oldest-inserted)
	// policy this would be evicted first even though it's the safest entry
	// to keep.
	if err := s.Revoke(context.Background(), "oldest-but-far-from-expiry", now.Add(24*time.Hour)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	// Inserted second, and closest to expiry: this is the entry the fixed
	// policy must evict.
	if err := s.Revoke(context.Background(), "newer-but-soonest-to-expire", now.Add(time.Second)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := s.Revoke(context.Background(), "filler-1", now.Add(12*time.Hour)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := s.Revoke(context.Background(), "filler-2", now.Add(6*time.Hour)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	// Store is now at MaxEntries (4). One more revoke forces an eviction.
	if err := s.Revoke(context.Background(), "fresh", now.Add(time.Hour)); err != nil {
		t.Fatalf("Revoke() at capacity error = %v", err)
	}

	if len(s.revoked) != 4 {
		t.Fatalf("revoked len = %d, want 4", len(s.revoked))
	}
	if _, ok := s.revoked["newer-but-soonest-to-expire"]; ok {
		t.Error("entry closest to expiry survived capacity eviction, want it evicted")
	}
	if _, ok := s.revoked["oldest-but-far-from-expiry"]; !ok {
		t.Error("oldest-inserted entry was evicted even though it was far from expiry")
	}
	if _, ok := s.revoked["filler-1"]; !ok {
		t.Error("filler-1 was unexpectedly evicted")
	}
	if _, ok := s.revoked["filler-2"]; !ok {
		t.Error("filler-2 was unexpectedly evicted")
	}
	if revoked, err := s.IsRevoked(context.Background(), "fresh"); err != nil || !revoked {
		t.Errorf("IsRevoked(fresh) = %v, %v, want true, nil", revoked, err)
	}
}

// TestMassRevocationBurst hammers Revoke well past MaxEntries in a tight
// loop (simulating a bulk logout / token rotation event) and asserts the
// store doesn't panic or corrupt its internal state: the map and the
// expiry heap must stay the same size, and every entry that IsRevoked
// reports as present must actually still be tracked and correctly reported
// as revoked.
func TestMassRevocationBurst(t *testing.T) {
	t.Parallel()

	const maxEntries = 64
	const burst = 5000

	si, err := New(Options{MaxEntries: maxEntries})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = si.Close() })

	s, ok := si.(*store)
	if !ok {
		t.Fatalf("New() returned %T, want *store", si)
	}

	now := time.Now()
	ctx := context.Background()
	jtis := make([]string, burst)
	for i := 0; i < burst; i++ {
		jti := "burst-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))
		jtis[i] = jti
		// Vary expiry so eviction has real choices to make, mixing
		// near-expiry and far-from-expiry entries.
		until := now.Add(time.Duration(i%1000+1) * time.Second)
		if err := s.Revoke(ctx, jti, until); err != nil {
			t.Fatalf("Revoke(%d) error = %v", i, err)
		}
	}

	s.mu.Lock()
	mapLen := len(s.revoked)
	heapLen := len(s.byExpiry)
	s.mu.Unlock()

	if mapLen > maxEntries {
		t.Fatalf("revoked map len = %d, want <= %d", mapLen, maxEntries)
	}
	if heapLen != mapLen {
		t.Fatalf("byExpiry heap len = %d, want == revoked map len %d (state corrupted)", heapLen, mapLen)
	}

	// Every jti still tracked in the map must be correctly reported as
	// revoked by IsRevoked; every evicted jti must correctly report false.
	for _, jti := range jtis {
		s.mu.Lock()
		until, present := s.revoked[jti]
		s.mu.Unlock()

		revoked, err := s.IsRevoked(ctx, jti)
		if err != nil {
			t.Fatalf("IsRevoked(%q) error = %v", jti, err)
		}

		want := present && now.Before(until)
		if revoked != want {
			t.Errorf("IsRevoked(%q) = %v, want %v (present=%v)", jti, revoked, want, present)
		}
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
	if err := s.Revoke(context.Background(), "old", past); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := s.Revoke(context.Background(), "live", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	s.pruneOnce(time.Now())

	if _, ok := s.revoked["old"]; ok {
		t.Error("pruneOnce kept expired entry")
	}
	if _, ok := s.revoked["live"]; !ok {
		t.Error("pruneOnce dropped live entry")
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

	if err := s.Revoke(context.Background(), "old-jti", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := s.Revoke(context.Background(), "live", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	fired <- time.Now()

	eventually(t, time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, gone := s.revoked["old-jti"]
		return !gone
	}, "pruneOnce to remove expired entry")

	s.mu.Lock()
	_, live := s.revoked["live"]
	s.mu.Unlock()

	if !live {
		t.Error("pruneOnce dropped live entry")
	}
}
