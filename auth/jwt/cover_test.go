package jwt

// Coverage note: all formerly-defensive lines are now resolved.
//   - sign failure: pruned — HMAC-SHA256 over validated []byte key with
//     JSON-serializable claims is infallible by construction.
//   - claims type assertion (verify + revoke): pruned — ParseWithClaims
//     into &MapClaims{} guarantees concrete type; nil error implies Valid.
//   - ticker-loop pruneOnce arm: covered via newTicker seam (see
//     TestCoverPrunerTickFiresPruneOnce).

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/auth"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("CSPRNG broken") }

func TestCoverVerifyNoneAlgRejected(t *testing.T) {
	t.Parallel()

	a := newTestAuth(t, baseOpts())
	ctx := context.Background()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`))
	_, err := a.Verify(ctx, header+"."+body+".")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(none-alg) = %v, want ErrInvalidToken", err)
	}
}

func TestCoverKeyfuncRejectsNonHS256(t *testing.T) {
	t.Parallel()

	ai, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ad, ok := ai.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ai)
	}
	t.Cleanup(func() { _ = ai.Close() })

	// White-box: the keyfunc alg guard is defense-in-depth behind
	// WithValidMethods, so it is exercised directly with unsigned
	// tokens of foreign algorithms.
	for _, method := range []jwtv5.SigningMethod{
		jwtv5.SigningMethodNone,
		jwtv5.SigningMethodHS384,
		jwtv5.SigningMethodRS256,
	} {
		tok := &jwtv5.Token{Method: method, Header: map[string]any{"alg": method.Alg()}}
		if _, err := ad.keyfunc(tok); err == nil {
			t.Errorf("keyfunc(%s) = nil, want rejection", method.Alg())
		}
	}
}

func TestCoverIssueCSPRNCFailure(t *testing.T) {
	// Not parallel: swaps the package CSPRNG source.
	old := randReader
	randReader = brokenReader{}
	defer func() { randReader = old }()

	a := newTestAuth(t, baseOpts())
	_, err := a.Issue(context.Background(), "sub", nil, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "jti") {
		t.Fatalf("Issue(broken CSPRNG) = %v, want jti error", err)
	}
}

func TestCoverVerifyNoCustomClaimsNil(t *testing.T) {
	t.Parallel()

	a := newTestAuth(t, baseOpts())
	ctx := context.Background()

	tok, err := a.Issue(ctx, "sub", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Custom != nil {
		t.Errorf("Custom = %v, want nil for claimless token", got.Custom)
	}
}

func TestCoverRevokeGarbage(t *testing.T) {
	t.Parallel()

	a := newTestAuth(t, baseOpts())
	if err := a.Revoke(context.Background(), "garbage"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(garbage) = %v, want ErrInvalidToken", err)
	}
}

func fillRevoked(ad *adapter, n int, until time.Time) {
	for i := 0; i < n; i++ {
		tok := fmt.Sprintf("tok-%08d-padpadpadpad", i)
		ad.revoked[tok] = until
		ad.order = append(ad.order, tok)
	}
}

func TestCoverCapacityEvictsOldest(t *testing.T) {
	t.Parallel()

	ai, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ad, ok := ai.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ai)
	}
	t.Cleanup(func() { _ = ai.Close() })

	future := time.Now().Add(time.Hour)
	fillRevoked(ad, maxRevoked, future)

	tok, err := ai.Issue(context.Background(), "fresh", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := ai.Revoke(context.Background(), tok.Value); err != nil {
		t.Fatalf("Revoke at capacity: %v", err)
	}
	if len(ad.revoked) != maxRevoked {
		t.Fatalf("revoked len = %d, want %d (evicted oldest)", len(ad.revoked), maxRevoked)
	}
	if _, ok := ad.revoked["tok-00000000-padpadpadpad"]; ok {
		t.Error("oldest entry survived capacity eviction")
	}
}

func TestCoverPruneExpiredDirect(t *testing.T) {
	t.Parallel()

	ai, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ad, ok := ai.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ai)
	}
	t.Cleanup(func() { _ = ai.Close() })

	past := time.Now().Add(-time.Minute)
	ad.revoked["old"] = past
	ad.revoked["live"] = time.Now().Add(time.Hour)

	ad.pruneOnce(time.Now())

	if _, ok := ad.revoked["old"]; ok {
		t.Error("pruneOnce kept expired entry")
	}
	if _, ok := ad.revoked["live"]; !ok {
		t.Error("pruneOnce dropped live entry")
	}
}

func TestCoverEvictOldestCompacts(t *testing.T) {
	t.Parallel()

	ai, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ad, ok := ai.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ai)
	}
	t.Cleanup(func() { _ = ai.Close() })

	// 2000 stale order entries, none in the map: the loop runs to the
	// end, then the compact branch (head>1024 && head>len/2) fires.
	for i := 0; i < 2000; i++ {
		ad.order = append(ad.order, strings.Repeat("z", 8)+string(rune('a'+i%26)))
	}
	ad.evictOldestLocked()

	if ad.head != 0 {
		t.Errorf("head = %d, want 0 after compact", ad.head)
	}
	if len(ad.order) != 0 {
		t.Errorf("order len = %d, want 0 after compact", len(ad.order))
	}
}

func TestCoverPrunerTickFiresPruneOnce(t *testing.T) {
	// Not parallel: swaps global newTicker seam.

	fired := make(chan time.Time, 1)
	old := newTicker
	newTicker = func(time.Duration) *time.Ticker { return &time.Ticker{C: fired} }
	defer func() { newTicker = old }()

	ai, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ad, ok := ai.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ai)
	}
	t.Cleanup(func() { _ = ai.Close() })

	// Plant an expired entry that pruneOnce should remove.
	ad.mu.Lock()
	ad.revoked["old-tok"] = time.Now().Add(-time.Minute)
	// Plant a live entry that should survive.
	ad.revoked["live"] = time.Now().Add(time.Hour)
	ad.mu.Unlock()

	// Fire the ticker once.
	fired <- time.Now()

	// Poll until the expired entry is pruned (or timeout).
	deadline := time.After(time.Second)
	var live bool
	for {
		ad.mu.RLock()
		_, gone := ad.revoked["old-tok"]
		_, live = ad.revoked["live"]
		ad.mu.RUnlock()
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
