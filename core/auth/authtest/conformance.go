// Package authtest provides the conformance kit third-party auth adapters run to prove backend parity.
package authtest

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
)

const (
	// DefaultRoundTripTTL is the TTL round-trip cases issue with: long
	// enough to survive the verify that follows without clock risk.
	DefaultRoundTripTTL = time.Hour
	// DefaultExpiryTTL is the short TTL the expiry case issues with before
	// polling for failure.
	DefaultExpiryTTL = 50 * time.Millisecond
	// DefaultExpiryTimeout bounds how long expiry polls wait before failing.
	DefaultExpiryTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between expiry-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

// Conformance verifies factory-built auth backends implement the
// auth.Auth contract: issue/verify/revoke round-trip, empty-subject
// rejection, unserializable-custom-claim handling, expiry mapping, and
// idempotent revoke. Each subtest takes a fresh instance from factory so
// cases stay isolated. Expiry waits poll with a deadline; they never
// synchronize with time.Sleep and never touch the network.
func Conformance(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	t.Run("RoundTrip", func(t *testing.T) { conformanceRoundTrip(t, factory) })
	t.Run("EmptySubject", func(t *testing.T) { conformanceEmptySubject(t, factory) })
	t.Run("UnserializableClaim", func(t *testing.T) { conformanceUnserializableClaim(t, factory) })
	t.Run("Expiry", func(t *testing.T) { conformanceExpiry(t, factory) })
	t.Run("Revoke", func(t *testing.T) { conformanceRevoke(t, factory) })
}

func conformanceRoundTrip(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	custom := map[string]any{"team": "eng", "tier": "pro"}
	tok, err := a.Issue(ctx, "alice", custom, DefaultRoundTripTTL)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	// Issued tokens are never empty: both adapters return Token{} only
	// on error (jwt.go returns Token{} on every failure branch;
	// session.go likewise).
	if tok.Value == "" {
		t.Fatal("Issue() token Value is empty")
	}
	// Both adapters stamp ExpiresAt as now+ttl (jwt.go from time.Now at
	// issue; session.go from the store Create expiry).
	if tok.ExpiresAt.IsZero() {
		t.Error("Issue() ExpiresAt is zero")
	}

	claims, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.Subject != "alice" {
		t.Errorf("Verify() Subject = %q, want alice", claims.Subject)
	}
	if claims.Custom["team"] != "eng" || claims.Custom["tier"] != "pro" {
		t.Errorf("Verify() Custom = %v, want team/tier preserved", claims.Custom)
	}
	if claims.ExpiresAt.IsZero() {
		t.Error("Verify() Claims.ExpiresAt is zero")
	}
}

func conformanceEmptySubject(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	// Empty subjects never authenticate: jwt.go Issue rejects with
	// wrapped ErrInvalidToken and session.go Issue returns bare
	// ErrInvalidToken; both return an empty Token on this path.
	tok, err := a.Issue(ctx, "", nil, DefaultRoundTripTTL)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Issue(\"\") err = %v, want ErrInvalidToken", err)
	}
	if tok.Value != "" {
		t.Errorf("Issue(\"\") token Value = %q, want empty (no credential minted on failure)", tok.Value)
	}
	// Empty tokens never verify: jwt.go and session.go both return
	// ErrInvalidToken before any backend lookup.
	if _, err := a.Verify(ctx, ""); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Verify(\"\") err = %v, want ErrInvalidToken", err)
	}
}

func conformanceUnserializableClaim(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	custom := map[string]any{"bad": func() {}}
	tok, err := a.Issue(ctx, "alice", custom, DefaultRoundTripTTL)
	if err == nil {
		// Backend stores claims opaquely (the session adapter persists
		// the envelope map without JSON encoding), so unserializable
		// values round-trip in-process. The contract then is only that
		// the identity still verifies.
		claims, verr := a.Verify(ctx, tok.Value)
		if verr != nil {
			t.Fatalf("Verify() after opaque Issue error = %v", verr)
		}
		if claims.Subject != "alice" {
			t.Errorf("Verify() Subject = %q, want alice", claims.Subject)
		}
		if rerr := a.Revoke(ctx, tok.Value); rerr != nil {
			t.Errorf("Revoke() error = %v, want nil", rerr)
		}
		return
	}
	// JSON-signing backends fail here instead of minting a token: jwt.go
	// wraps the signing failure with ErrInvalidToken via errors.Join and
	// returns Token{}, so no empty credential escapes.
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Issue(unserializable) err = %v, want ErrInvalidToken", err)
	}
	if tok.Value != "" {
		t.Errorf("Issue(unserializable) token Value = %q, want empty (no credential minted on failure)", tok.Value)
	}
}

func conformanceExpiry(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	tok, err := a.Issue(ctx, "alice", nil, DefaultExpiryTTL)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	deadline := time.Now().Add(DefaultExpiryTimeout)
	var lastErr error
	for {
		_, lastErr = a.Verify(ctx, tok.Value)
		if lastErr != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Verify(short-ttl token) still succeeds past deadline, want expiry failure")
		}
		timer := time.NewTimer(DefaultPollInterval)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatal("test context done waiting for expiry")
		case <-timer.C:
		}
	}
	// Distinct-expiry backends report ErrTokenExpired (jwt.go maps
	// jwtv5.ErrTokenExpired; session.go maps a stale envelope past its
	// 1m leeway). Store-expiry backends report ErrInvalidToken instead:
	// session.go maps a reaped record (Get -> session.ErrNotFound) to
	// ErrInvalidToken because miss and expiry are indistinguishable by
	// design. Both are fail-closed; only success or revocation here is
	// a contract violation.
	if !errors.Is(lastErr, auth.ErrTokenExpired) && !errors.Is(lastErr, auth.ErrInvalidToken) {
		t.Errorf("Verify(expired) err = %v, want ErrTokenExpired or ErrInvalidToken", lastErr)
	}
}

func conformanceRevoke(t *testing.T, factory func(t *testing.T) auth.Auth) {
	t.Helper()

	ctx := t.Context()
	a := factory(t)

	tok, err := a.Issue(ctx, "alice", nil, DefaultRoundTripTTL)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if err := a.Revoke(ctx, tok.Value); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}
	// Post-revoke verification always fails, but the sentinel differs:
	// jwt.go consults its revocation store and reports ErrTokenRevoked,
	// while session.go deletes the record so the next Verify is a miss
	// (ErrInvalidToken). Both are fail-closed.
	if _, err := a.Verify(ctx, tok.Value); !errors.Is(err, auth.ErrTokenRevoked) && !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Verify(revoked) err = %v, want ErrTokenRevoked or ErrInvalidToken", err)
	}
	// Revocation is idempotent per the Auth interface: revoking an
	// unknown or already-revoked token returns nil (jwt.go re-records
	// the jti; session.go deletes idempotently per Store contract).
	if err := a.Revoke(ctx, tok.Value); err != nil {
		t.Errorf("Revoke(again) error = %v, want nil (idempotent)", err)
	}
	// Empty tokens are rejected, not silently accepted: jwt.go and
	// session.go both wrap ErrInvalidToken on this path.
	if err := a.Revoke(ctx, ""); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Revoke(\"\") err = %v, want ErrInvalidToken", err)
	}
}
