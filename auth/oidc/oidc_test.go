package oidc_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/auth/oidc"
)

// NOTE: tests use 2048-bit RSA keys (fast to generate, ~ms) so gosec
// key-size rules hold even in tests. Each key lives only for its test
// and signs only test tokens.

// fakeIDP is a hermetic OIDC provider: discovery document + JWKS.
// No external network (no Google); per-test server so t.Parallel is safe.
type fakeIDP struct {
	srv *httptest.Server
	key *rsa.PrivateKey
	kid string
}

func b64u(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test key: %s", err)
	}

	idp := &fakeIDP{key: key, kid: "test-key-1"}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   idp.srv.URL,
			"jwks_uri": idp.srv.URL + "/keys",
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": idp.kid,
				"use": "sig",
				"alg": "RS256",
				"n":   b64u(key.N.Bytes()),
				"e":   b64u(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})

	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

// mintToken hand-rolls an RS256 JWT with stdlib only: no JWT signer dep.
func mintToken(t *testing.T, key *rsa.PrivateKey, kid, iss, aud string, exp time.Time, extra map[string]any) string {
	t.Helper()

	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %s", err)
	}
	payloadMap := map[string]any{
		"iss": iss,
		"aud": aud,
		"sub": "user-123",
		"exp": exp.Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extra {
		payloadMap[k] = v
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		t.Fatalf("marshal payload: %s", err)
	}

	input := b64u(header) + "." + b64u(payload)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign token: %s", err)
	}
	return input + "." + b64u(sig)
}

func newAdapter(t *testing.T, idp *fakeIDP) auth.Auth {
	t.Helper()

	opts := auth.Options{}
	opts.OIDC.Issuer = idp.srv.URL
	opts.OIDC.ClientID = "test-client"
	a, err := oidc.New(opts)
	if err != nil {
		t.Fatalf("oidc.New: %s", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %s", err)
		}
	})
	return a
}

func TestVerify_ValidRoundtrip(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	raw := mintToken(t, idp.key, idp.kid, idp.srv.URL, "test-client", exp,
		map[string]any{"email": "u@example.com", "email_verified": true, "name": "U Example"})

	got, err := a.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("Verify: %s", err)
	}
	if got.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", got.Subject)
	}
	if got.Custom["email"] != "u@example.com" {
		t.Errorf("Custom[email] = %v", got.Custom["email"])
	}
	if got.Custom["email_verified"] != true {
		t.Errorf("Custom[email_verified] = %v", got.Custom["email_verified"])
	}
	if got.Custom["name"] != "U Example" {
		t.Errorf("Custom[name] = %v", got.Custom["name"])
	}
	if got.ExpiresAt.Unix() != exp.Unix() {
		t.Errorf("ExpiresAt = %s, want %s", got.ExpiresAt, exp)
	}
}

func TestVerify_Expired(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	raw := mintToken(t, idp.key, idp.kid, idp.srv.URL, "test-client",
		time.Now().Add(-time.Hour), nil)

	_, err := a.Verify(context.Background(), raw)
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("Verify err = %v, want ErrTokenExpired", err)
	}
}

func TestVerify_WrongAudience(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	raw := mintToken(t, idp.key, idp.kid, idp.srv.URL, "other-client",
		time.Now().Add(time.Hour), nil)

	_, err := a.Verify(context.Background(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_WrongIssuer(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	raw := mintToken(t, idp.key, idp.kid, "https://evil.example.com", "test-client",
		time.Now().Add(time.Hour), nil)

	_, err := a.Verify(context.Background(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_BadSignature(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other key: %s", err)
	}
	// Same kid, different key: signature check must fail.
	raw := mintToken(t, other, idp.kid, idp.srv.URL, "test-client",
		time.Now().Add(time.Hour), nil)

	_, err = a.Verify(context.Background(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_Malformed(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	for _, raw := range []string{"not.a.jwt", "abc", "a.b", strings.Repeat("x", 512)} {
		_, err := a.Verify(context.Background(), raw)
		if !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("Verify(%q) err = %v, want ErrInvalidToken", raw, err)
		}
		if err != nil && strings.Contains(err.Error(), raw) {
			t.Errorf("Verify(%q) error echoes token bytes", raw)
		}
	}
}

func TestVerify_Empty(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	_, err := a.Verify(context.Background(), "")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(\"\") err = %v, want ErrInvalidToken", err)
	}
}

func TestIssue_NotSupported(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	_, err := a.Issue(context.Background(), "user-123", nil, time.Hour)
	if !errors.Is(err, auth.ErrNotSupported) {
		t.Fatalf("Issue err = %v, want ErrNotSupported", err)
	}
}

func TestRevoke_NotSupported(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	if err := a.Revoke(context.Background(), "sometoken"); !errors.Is(err, auth.ErrNotSupported) {
		t.Fatalf("Revoke err = %v, want ErrNotSupported", err)
	}
}

func TestNew_InvalidOptions(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)

	cases := map[string]func(*auth.Options){
		"empty issuer":   func(o *auth.Options) { o.OIDC.Issuer = ""; o.OIDC.ClientID = "c" },
		"empty clientID": func(o *auth.Options) { o.OIDC.Issuer = idp.srv.URL; o.OIDC.ClientID = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts := auth.Options{}
			mutate(&opts)
			_, err := oidc.New(opts)
			var invErr *auth.InvalidOptionsError
			if !errors.As(err, &invErr) {
				t.Fatalf("New err = %v, want *InvalidOptionsError", err)
			}
			if !errors.Is(err, auth.ErrInvalidOptions) {
				t.Fatalf("New err = %v, want ErrInvalidOptions in chain", err)
			}
		})
	}
}

func TestNew_NegativeTimeout(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	opts := auth.Options{}
	opts.OIDC.Issuer = idp.srv.URL
	opts.OIDC.ClientID = "test-client"
	opts.OIDC.Timeout = -time.Second

	if _, err := oidc.New(opts); err == nil {
		t.Fatal("New with negative timeout: want error, got nil")
	}
}

func TestNew_DiscoveryFailure(t *testing.T) {
	t.Parallel()

	// Closed server: connection refused on discovery.
	srv := httptest.NewServer(http.NewServeMux())
	url := srv.URL
	srv.Close()

	opts := auth.Options{}
	opts.OIDC.Issuer = url
	opts.OIDC.ClientID = "test-client"

	if _, err := oidc.New(opts); err == nil {
		t.Fatal("New with dead issuer: want error, got nil")
	}
}

func TestNew_Timeout(t *testing.T) {
	t.Parallel()

	// Hanging discovery endpoint: New must give up after Timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	opts := auth.Options{}
	opts.OIDC.Issuer = srv.URL
	opts.OIDC.ClientID = "test-client"
	opts.OIDC.Timeout = 200 * time.Millisecond

	start := time.Now()
	_, err := oidc.New(opts)
	if err == nil {
		t.Fatal("New with hanging issuer: want error, got nil")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("New took %s, timeout not applied", took)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %s", err)
	}
}
