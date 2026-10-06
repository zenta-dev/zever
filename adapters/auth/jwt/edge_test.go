package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/auth/revocation"
)

// TestEdgeVerify_concurrent proves the adapter and its revocation store are
// safe for concurrent verification of a shared token.
func TestEdgeVerify_concurrent(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	ctx := t.Context()

	tok, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := a.Verify(ctx, tok.Value); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Verify: %v", err)
	}
}

// TestEdgeVerify_malformedToken proves garbage input fails closed with
// ErrInvalidToken and never echoes the token bytes back in the error.
func TestEdgeVerify_malformedToken(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())

	for _, raw := range []string{"", "not-a-token", "a.b", "a.b.c.d", strings.Repeat("x", 4096)} {
		_, err := a.Verify(t.Context(), raw)
		if !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("Verify(%q) err = %v, want ErrInvalidToken", raw, err)
		}
		if raw != "" && err != nil && strings.Contains(err.Error(), raw) {
			t.Errorf("Verify(%q) error echoes token bytes", raw)
		}
	}
}

// TestEdgeVerify_nonStringJTI proves a token whose jti claim is not a string
// is rejected instead of being treated as absent (which would skip the
// revocation check).
func TestEdgeVerify_nonStringJTI(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "test-aud",
		"jti": 12345,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(non-string jti) err = %v, want ErrInvalidToken", err)
	}
}

// TestEdgeRevoke_noJTI proves a validly-signed token without a jti claim is
// rejected with ErrInvalidToken wrapping revocation.ErrNoJTI.
func TestEdgeRevoke_noJTI(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	err := a.Revoke(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke(no jti) err = %v, want ErrInvalidToken", err)
	}
	if !errors.Is(err, revocation.ErrNoJTI) {
		t.Fatalf("Revoke(no jti) err = %v, want wrap revocation.ErrNoJTI", err)
	}
}

// TestEdgeVerify_algConfusionRS256 proves a token whose header alg is RS256
// (or any non-HS256 value) is rejected by the keyfunc before any signature
// work happens.
func TestEdgeVerify_algConfusionRS256(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, jwtv5.MapClaims{
		"sub": "mallory",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString(RS256): %v", err)
	}

	_, err = a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(RS256 alg) err = %v, want ErrInvalidToken", err)
	}
}

// TestEdgeIssue_nilCustom proves a nil custom-claims map mints and verifies
// cleanly, returning nil (not empty) Custom on Verify.
func TestEdgeIssue_nilCustom(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	ctx := t.Context()

	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue(nil custom) err = %v", err)
	}

	got, err := a.Verify(ctx, tok.Value)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if got.Custom != nil {
		t.Fatalf("Verify Custom = %v, want nil", got.Custom)
	}
}
