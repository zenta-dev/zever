package jwt

// Coverage note: all formerly-defensive lines are now resolved.
//   - sign failure: pruned — HMAC-SHA256 over validated []byte key with
//     JSON-serializable claims is infallible by construction.
//   - claims type assertion (verify + revoke): pruned — ParseWithClaims
//     into &MapClaims{} guarantees concrete type; nil error implies Valid.
//   - revocation cap/prune/ticker coverage lives in
//     core/auth/revocation/memory now that the adapter delegates to a
//     revocation.Store instead of holding the map itself.

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("CSPRNG broken") }

// erroringRevocationStore fails every call, exercising the store-error
// pass-through branches in Verify and Revoke.
type erroringRevocationStore struct{}

func (erroringRevocationStore) Revoke(context.Context, string, time.Time) error {
	return errors.New("revocation backend down")
}

func (erroringRevocationStore) IsRevoked(context.Context, string) (bool, error) {
	return false, errors.New("revocation backend down")
}

func (erroringRevocationStore) Close() error { return nil }

func TestCoverVerifyRevocationStoreError(t *testing.T) {
	t.Parallel()

	opts := baseOpts()
	opts.JWT.RevocationStore = erroringRevocationStore{}
	a := freshAdapter(t, opts)
	ctx := t.Context()

	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := a.Verify(ctx, tok.Value); err == nil {
		t.Fatal("Verify() with failing revocation store succeeded, want error")
	}
}

func TestCoverRevokeRevocationStoreError(t *testing.T) {
	t.Parallel()

	opts := baseOpts()
	opts.JWT.RevocationStore = erroringRevocationStore{}
	a := freshAdapter(t, opts)
	ctx := t.Context()

	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := a.Revoke(ctx, tok.Value); err == nil {
		t.Fatal("Revoke() with failing revocation store succeeded, want error")
	}
}

func TestCoverVerifyNoneAlgRejected(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	ctx := t.Context()

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

	a := freshAdapter(t, baseOpts())
	_, err := a.Issue(t.Context(), "sub", nil, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "jti") {
		t.Fatalf("Issue(broken CSPRNG) = %v, want jti error", err)
	}
}

func TestCoverVerifyNoCustomClaimsNil(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	ctx := t.Context()

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

	a := freshAdapter(t, baseOpts())
	if err := a.Revoke(t.Context(), "garbage"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(garbage) = %v, want ErrInvalidToken", err)
	}
}

func TestCoverRevokeNoJTI(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, jwtv5.MapClaims{
		"sub": "x",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if err := a.Revoke(t.Context(), raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(no jti) = %v, want ErrInvalidToken", err)
	}
}

func TestCoverVerifyNoJTINeverRevoked(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	got, err := a.Verify(t.Context(), raw)
	if err != nil {
		t.Fatalf("Verify(no jti) error = %v, want nil", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Verify(no jti) subject = %q, want alice", got.Subject)
	}
}
