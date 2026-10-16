package jwt

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

func newClockAdapter(t *testing.T) (*adapter, *fakeClock) {
	t.Helper()

	a, ok := freshAdapter(t, baseOpts()).(*adapter)
	if !ok {
		t.Fatal("New() did not return *adapter")
	}

	clk := &fakeClock{t: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	a.now = clk.now

	return a, clk
}

func TestClock_IssueAndVerifyExpiry(t *testing.T) {
	t.Parallel()

	a, clk := newClockAdapter(t)

	tok, err := a.Issue(t.Context(), "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if want := clk.now().Add(time.Minute).Truncate(time.Second); !tok.ExpiresAt.Truncate(time.Second).Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", tok.ExpiresAt, want)
	}

	clk.advance(30 * time.Second)

	if _, err := a.Verify(t.Context(), tok.Value); err != nil {
		t.Fatalf("Verify() before expiry error = %v", err)
	}

	clk.advance(31 * time.Second)

	if _, err := a.Verify(t.Context(), tok.Value); !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("Verify() after expiry error = %v, want ErrTokenExpired", err)
	}
}

func TestClock_RevokeUsesInjectedClock(t *testing.T) {
	t.Parallel()

	store := newFakeRevocationStore()
	opts := baseOpts()
	opts.JWT.RevocationStore = store

	a, ok := freshAdapter(t, opts).(*adapter)
	if !ok {
		t.Fatal("New() did not return *adapter")
	}

	clk := &fakeClock{t: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	a.now = clk.now

	tok, err := a.Issue(t.Context(), "alice", nil, time.Hour)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	clk.advance(10 * time.Minute)

	if err := a.Revoke(t.Context(), tok.Value); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	// Token still live on the fake clock: the revocation lasts until its exp.
	store.mu.Lock()
	defer store.mu.Unlock()

	for _, until := range store.revoked {
		if want := tok.ExpiresAt.Truncate(time.Second); !until.Equal(want) {
			t.Errorf("revoked until %v, want %v", until, want)
		}
	}

	if len(store.revoked) != 1 {
		t.Fatalf("revoked %d entries, want 1", len(store.revoked))
	}
}
