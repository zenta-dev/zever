package jwt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/auth/jwt/revocation"
)

// fakeRevocationStore records calls, so tests can assert the adapter wires
// the injected auth.JWTOptions.RevocationStore through instead of building
// its own.
type fakeRevocationStore struct {
	mu        sync.Mutex
	revoked   map[string]time.Time
	closed    bool
	revokeN   int
	isRevokeN int
}

var _ revocation.Store = (*fakeRevocationStore)(nil)

func newFakeRevocationStore() *fakeRevocationStore {
	return &fakeRevocationStore{revoked: make(map[string]time.Time)}
}

func (f *fakeRevocationStore) Revoke(_ context.Context, jti string, until time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeN++
	f.revoked[jti] = until
	return nil
}

func (f *fakeRevocationStore) IsRevoked(_ context.Context, jti string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isRevokeN++
	until, ok := f.revoked[jti]
	if !ok {
		return false, nil
	}
	return time.Now().Before(until), nil
}

func (f *fakeRevocationStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

const testSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func baseOpts() auth.Options {
	return auth.Options{JWT: auth.JWTOptions{
		Secret:   testSecret,
		Issuer:   "test-iss",
		Audience: "test-aud",
		MaxTTL:   time.Hour,
	}}
}

func newTestAuth(t *testing.T, opts auth.Options) auth.Auth {
	t.Helper()
	a, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return a
}

func TestNew_RejectsShortSecret(t *testing.T) {
	t.Parallel()
	_, err := New(auth.Options{JWT: auth.JWTOptions{Secret: "short"}})
	if err == nil {
		t.Fatal("New() with short secret succeeded, want error")
	}
	var inv *auth.InvalidOptionsError
	if !errors.As(err, &inv) {
		t.Fatalf("New() error = %T (%v), want *InvalidOptionsError", err, err)
	}
}

func TestNew_RejectsEmptySecret(t *testing.T) {
	t.Parallel()
	_, err := New(auth.Options{})
	if err == nil {
		t.Fatal("New() with empty secret succeeded, want error")
	}
	var inv *auth.InvalidOptionsError
	if !errors.As(err, &inv) {
		t.Fatalf("New() error = %T (%v), want *InvalidOptionsError", err, err)
	}
}

func TestNew_RejectsNegativeMaxTTL(t *testing.T) {
	t.Parallel()
	opts := baseOpts()
	opts.JWT.MaxTTL = -time.Second
	_, err := New(opts)
	if err == nil {
		t.Fatal("New() with negative MaxTTL succeeded, want error")
	}
	if !errors.Is(err, auth.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
	}
}

func TestNew_OK(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	if a == nil {
		t.Fatal("New() returned nil Auth")
	}
}

func TestIssueVerify_Roundtrip(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	ctx := t.Context()

	custom := map[string]any{"role": "admin", "level": "7"}
	tok, err := a.Issue(ctx, "alice", custom, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if tok.Value == "" {
		t.Fatal("Issue() returned empty token value")
	}
	if tok.ExpiresAt.IsZero() {
		t.Fatal("Issue() returned zero ExpiresAt")
	}

	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Verify() subject = %q, want alice", got.Subject)
	}
	if got.Custom["role"] != "admin" {
		t.Fatalf("Verify() custom role = %v, want admin", got.Custom["role"])
	}
}

func TestIssueVerify_CustomCloneIndependence(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	ctx := t.Context()

	custom := map[string]any{"tags": []string{"a", "b"}}
	tok, err := a.Issue(ctx, "bob", custom, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	// Mutating caller map after Issue must not affect stored claims.
	srcTags, ok := custom["tags"].([]string)
	if !ok {
		t.Fatalf("custom tags type = %T, want []string", custom["tags"])
	}
	srcTags[0] = "MUT"

	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	// JSON round-trip decodes arrays as []any.
	tags, ok := got.Custom["tags"].([]any)
	if !ok {
		t.Fatalf("Verify() tags type = %T, want []any", got.Custom["tags"])
	}
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Fatalf("Verify() tags = %v, want [a b]", tags)
	}
	// Mutating returned claims must not affect subsequent Verify.
	tags[0] = "MUT"
	got.Custom["role"] = "MUT"
	got2, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if _, ok := got2.Custom["role"]; ok {
		t.Fatal("Verify() leaked mutation into stored claims")
	}
}

func TestIssue_EmptySubject(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	_, err := a.Issue(t.Context(), "", nil, time.Minute)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Issue() error = %v, want ErrInvalidToken", err)
	}
}

func TestIssue_NonPositiveTTL(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := a.Issue(t.Context(), "x", nil, ttl); !errors.Is(err, auth.ErrInvalidToken) {
			t.Fatalf("Issue(ttl=%v) error = %v, want ErrInvalidToken", ttl, err)
		}
	}
}

func TestIssue_ExceedsMaxTTL(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	_, err := a.Issue(t.Context(), "x", nil, 2*time.Hour)
	if err == nil {
		t.Fatal("Issue() with ttl > MaxTTL succeeded, want error")
	}
}

func TestIssue_RejectsClaimCollision(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	for _, k := range []string{"sub", "iss", "aud", "exp", "iat", "jti"} {
		_, err := a.Issue(t.Context(), "x", map[string]any{k: "boom"}, time.Minute)
		if !errors.Is(err, auth.ErrInvalidToken) {
			t.Fatalf("Issue(custom[%q]) error = %v, want ErrInvalidToken", k, err)
		}
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	_, err := a.Verify(t.Context(), "")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	tok, err := a.Issue(t.Context(), "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	other, err := New(auth.Options{JWT: auth.JWTOptions{
		Secret:   "fedcba9876543210fedcba9876543210",
		Issuer:   "test-iss",
		Audience: "test-aud",
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })
	_, err = other.Verify(t.Context(), tok.Value)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify() with wrong secret error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	tok, err := a.Issue(t.Context(), "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	parts := strings.Split(tok.Value, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payload := parts[1]
	var flip byte = 'A'
	if payload[len(payload)/2] == 'A' {
		flip = 'B'
	}
	tampered := parts[0] + "." + payload[:len(payload)/2] + string(flip) + payload[len(payload)/2+1:] + "." + parts[2]
	_, err = a.Verify(t.Context(), tampered)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(tampered) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_NoneAlgRejected(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	none := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, jwtv5.MapClaims{
		"sub": "mallory",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	raw, err := none.SignedString(jwtv5.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString(none) error = %v", err)
	}
	_, err = a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(none-alg) error = %v, want ErrInvalidToken", err)
	}
}

func signManual(t *testing.T, claims jwtv5.MapClaims) string {
	t.Helper()
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	raw, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return raw
}

func TestVerify_Expired(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(-time.Minute).Unix(),
		"iat": time.Now().Add(-2 * time.Minute).Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("Verify(expired) error = %v, want ErrTokenExpired", err)
	}
}

func TestVerify_IssuerMismatch(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "other-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(wrong iss) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_StrictEmptyIssuer(t *testing.T) {
	t.Parallel()
	opts := auth.Options{JWT: auth.JWTOptions{Secret: testSecret}}
	a := newTestAuth(t, opts)
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "sneaky",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(iss set, adapter empty) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_AudienceMismatch(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "other-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(wrong aud) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_StrictEmptyAudience(t *testing.T) {
	t.Parallel()
	opts := auth.Options{JWT: auth.JWTOptions{Secret: testSecret, Issuer: "test-iss"}}
	a := newTestAuth(t, opts)
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "sneaky",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(aud set, adapter empty) error = %v, want ErrInvalidToken", err)
	}
}

func TestRevoke_VerifyAfterRevoke(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	ctx := t.Context()
	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if rerr := a.Revoke(ctx, tok.Value); rerr != nil {
		t.Fatalf("Revoke() error = %v", rerr)
	}
	_, err = a.Verify(ctx, tok.Value)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("Verify(revoked) error = %v, want ErrTokenRevoked", err)
	}
}

func TestRevoke_Idempotent(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	ctx := t.Context()
	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if rerr := a.Revoke(ctx, tok.Value); rerr != nil {
		t.Fatalf("Revoke() error = %v", rerr)
	}
	if rerr := a.Revoke(ctx, tok.Value); rerr != nil {
		t.Fatalf("Revoke() second call error = %v, want nil", rerr)
	}
}

func TestRevoke_BadInputs(t *testing.T) {
	t.Parallel()
	a := newTestAuth(t, baseOpts())
	ctx := t.Context()
	if err := a.Revoke(ctx, ""); err == nil {
		t.Fatal("Revoke(empty) succeeded, want error")
	}
	if err := a.Revoke(ctx, "not-a-token"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(garbage) error = %v, want ErrInvalidToken", err)
	}
	// Valid signature shape but wrong secret must fail.
	other := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, jwtv5.MapClaims{
		"sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, _ := other.SignedString([]byte("fedcba9876543210fedcba9876543210"))
	if err := a.Revoke(ctx, raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(foreign) error = %v, want ErrInvalidToken", err)
	}
}

func TestNew_UsesInjectedRevocationStore(t *testing.T) {
	t.Parallel()
	store := newFakeRevocationStore()
	opts := baseOpts()
	opts.JWT.RevocationStore = store
	a := newTestAuth(t, opts)
	ctx := t.Context()

	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if rerr := a.Revoke(ctx, tok.Value); rerr != nil {
		t.Fatalf("Revoke() error = %v", rerr)
	}
	if store.revokeN != 1 {
		t.Fatalf("injected store Revoke calls = %d, want 1", store.revokeN)
	}

	_, err = a.Verify(ctx, tok.Value)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("Verify() after Revoke via injected store = %v, want ErrTokenRevoked", err)
	}
	if store.isRevokeN == 0 {
		t.Fatal("injected store IsRevoked was never called")
	}
}

func TestClose_ClosesInjectedRevocationStore(t *testing.T) {
	t.Parallel()
	store := newFakeRevocationStore()
	opts := baseOpts()
	opts.JWT.RevocationStore = store
	a, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !store.closed {
		t.Fatal("Close() did not close the injected revocation store")
	}
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()
	a, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}
}
