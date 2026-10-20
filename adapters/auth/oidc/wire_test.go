package oidc_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/auth/oidc"
	"github.com/zenta-dev/zever/core/auth"
)

// The tests in this file pin the OIDC wire contract the adapter speaks:
// the discovery document and JWKS payload shapes it consumes from the IdP,
// and the exact claim keys it emits in auth.Claims.Custom. Key names are
// part of the provider contract — a renamed tag in profileClaims or a
// changed Custom key breaks every integration while still compiling.

// newStandardIDP serves the standard discovery and JWKS payload shapes
// with exact key names: the discovery document carries issuer and
// jwks_uri; the JWKS carries one RSA key with kty, kid, use, alg, n, e.
func newStandardIDP(tb testing.TB) (*httptest.Server, *rsa.PrivateKey) {
	tb.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("generate test key: %s", err)
	}

	mux := http.NewServeMux()

	var srv *httptest.Server

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   srv.URL,
			"jwks_uri": srv.URL + "/keys",
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": "test-key-1",
				"use": "sig",
				"alg": "RS256",
				"n":   b64u(key.N.Bytes()),
				"e":   b64u(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})

	srv = httptest.NewServer(mux)
	tb.Cleanup(srv.Close)

	return srv, key
}

// mintStandardToken hand-rolls an RS256 JWT over the standard discovery
// issuer, mirroring oidc_test.go's mintToken.
func mintStandardToken(tb testing.TB, key *rsa.PrivateKey, issuer, aud string, exp time.Time, extra map[string]any) string {
	tb.Helper()

	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": "test-key-1", "typ": "JWT"})
	if err != nil {
		tb.Fatalf("marshal header: %s", err)
	}

	payload := map[string]any{
		"iss": issuer,
		"aud": aud,
		"sub": "user-123",
		"exp": exp.Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extra {
		payload[k] = v
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		tb.Fatalf("marshal payload: %s", err)
	}

	input := b64u(header) + "." + b64u(payloadBytes)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		tb.Fatalf("sign token: %s", err)
	}

	return input + "." + b64u(sig)
}

func TestVerify_ClaimKeysAreExact(t *testing.T) {
	t.Parallel()

	idp, key := newStandardIDP(t)

	opts := auth.Options{}
	opts.OIDC.Issuer = idp.URL
	opts.OIDC.ClientID = "test-client"
	opts.OIDC.AllowInsecure = true

	a, err := oidc.New(opts)
	if err != nil {
		t.Fatalf("oidc.New: %s", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %s", err)
		}
	})

	// The ID token carries the three profile claims plus an unrelated
	// claim that must NOT leak into Custom.
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	raw := mintStandardToken(t, key, idp.URL, "test-client", exp,
		map[string]any{
			"email":          "u@example.com",
			"email_verified": true,
			"name":           "U Example",
			"admin":          true,
		})

	got, err := a.Verify(t.Context(), raw)
	if err != nil {
		t.Fatalf("Verify: %s", err)
	}

	want := map[string]any{
		"email":          "u@example.com",
		"email_verified": true,
		"name":           "U Example",
	}

	if len(got.Custom) != len(want) {
		t.Fatalf("Custom has %d keys %v, want exactly %d %v", len(got.Custom), got.Custom, len(want), want)
	}

	for k, v := range want {
		if got.Custom[k] != v {
			t.Errorf("Custom[%q] = %v, want %v", k, got.Custom[k], v)
		}
	}
}

func TestDiscoveryAndJWKSPayloadShape(t *testing.T) {
	t.Parallel()

	idp, key := newStandardIDP(t)

	opts := auth.Options{}
	opts.OIDC.Issuer = idp.URL
	opts.OIDC.ClientID = "test-client"
	opts.OIDC.AllowInsecure = true

	a, err := oidc.New(opts)
	if err != nil {
		t.Fatalf("oidc.New: %s", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %s", err)
		}
	})

	// Verify exercises discovery + JWKS against the standard payload
	// shapes above; success pins that the adapter consumes exactly these
	// member names.
	raw := mintStandardToken(t, key, idp.URL, "test-client",
		time.Now().Add(time.Hour).Truncate(time.Second),
		map[string]any{"email": "u@example.com"})

	if _, err := a.Verify(t.Context(), raw); err != nil {
		t.Fatalf("Verify against standard discovery/JWKS payloads: %s", err)
	}
}
