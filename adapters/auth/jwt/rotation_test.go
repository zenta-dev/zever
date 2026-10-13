package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
)

const (
	rotationSecretA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 bytes
	rotationSecretB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" // 32 bytes
)

// rotationOpts returns options with two keys, B active.
func rotationOpts() auth.Options {
	return auth.Options{JWT: auth.JWTOptions{
		Secret:   testSecret,
		Issuer:   "test-iss",
		Audience: "test-aud",
		MaxTTL:   time.Hour,
		Keys: []auth.JWTKey{
			{KID: "k1", Secret: rotationSecretA},
			{KID: "k2", Secret: rotationSecretB},
		},
		ActiveKID: "k2",
	}}
}

// signWithKID mints an HS256 token with an explicit kid header for tests.
func signWithKID(tb testing.TB, kid, secret string, claims jwtv5.MapClaims) string {
	tb.Helper()
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	raw, err := tok.SignedString([]byte(secret))
	if err != nil {
		tb.Fatalf("SignedString() error = %v", err)
	}
	return raw
}

func baseClaims() jwtv5.MapClaims {
	return jwtv5.MapClaims{
		"sub": "alice",
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

func TestVerify_RotationOldTokenStillVerifies(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, rotationOpts())
	ctx := t.Context()

	old := signWithKID(t, "k1", rotationSecretA, baseClaims())
	got, err := a.Verify(ctx, old)
	if err != nil {
		t.Fatalf("Verify(old kid token) error = %v, want nil", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Verify() subject = %q, want alice", got.Subject)
	}

	fresh, err := a.Issue(ctx, "bob", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	got, err = a.Verify(ctx, fresh.Value)
	if err != nil {
		t.Fatalf("Verify(new token) error = %v, want nil", err)
	}
	if got.Subject != "bob" {
		t.Fatalf("Verify() subject = %q, want bob", got.Subject)
	}
}

func TestVerify_UnknownKidRejected(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, rotationOpts())
	raw := signWithKID(t, "nope", rotationSecretA, baseClaims())
	if _, err := a.Verify(t.Context(), raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(unknown kid) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_LegacyKidlessTokenFallback(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, rotationOpts())
	ctx := t.Context()

	// Kid-less token signed with the legacy secret verifies.
	legacy := signWithKID(t, "", testSecret, baseClaims())
	got, err := a.Verify(ctx, legacy)
	if err != nil {
		t.Fatalf("Verify(legacy token) error = %v, want nil", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Verify() subject = %q, want alice", got.Subject)
	}

	// Keys-only adapter (no legacy secret) rejects kid-less tokens.
	keysOnly := rotationOpts()
	keysOnly.JWT.Secret = ""
	konly, err := New(keysOnly)
	if err != nil {
		t.Fatalf("New(keys-only) error = %v", err)
	}
	t.Cleanup(func() { _ = konly.Close() })
	if _, err := konly.Verify(ctx, legacy); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(kid-less, keys-only) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_NbfFutureRejected(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, baseOpts())
	claims := baseClaims()
	claims["nbf"] = time.Now().Add(time.Hour).Unix()
	raw := signWithKID(t, "", testSecret, claims)
	if _, err := a.Verify(t.Context(), raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(nbf future) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_NbfPastAccepted(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, baseOpts())
	claims := baseClaims()
	claims["nbf"] = time.Now().Add(-time.Minute).Unix()
	raw := signWithKID(t, "", testSecret, claims)
	got, err := a.Verify(t.Context(), raw)
	if err != nil {
		t.Fatalf("Verify(nbf past) error = %v, want nil", err)
	}
	if got.Subject != "alice" {
		t.Fatalf("Verify() subject = %q, want alice", got.Subject)
	}
}

func TestVerify_RS256ConfusionRejected(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, baseOpts())
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, baseClaims())
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString(RS256) error = %v", err)
	}
	if _, err := a.Verify(t.Context(), raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(RS256 token) error = %v, want ErrInvalidToken", err)
	}
}

func TestNew_RotationOptionsValidated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mut  func(*auth.Options)
	}{
		{"empty kid", func(o *auth.Options) {
			o.JWT.Keys = []auth.JWTKey{{KID: "", Secret: rotationSecretA}}
		}},
		{"short secret", func(o *auth.Options) {
			o.JWT.Keys = []auth.JWTKey{{KID: "k1", Secret: "short"}}
		}},
		{"duplicate kid", func(o *auth.Options) {
			o.JWT.Keys = []auth.JWTKey{
				{KID: "k1", Secret: rotationSecretA},
				{KID: "k1", Secret: rotationSecretB},
			}
		}},
		{"unknown active kid", func(o *auth.Options) {
			o.JWT.Keys = []auth.JWTKey{{KID: "k1", Secret: rotationSecretA}}
			o.JWT.ActiveKID = "nope"
		}},
		{"active kid without keys", func(o *auth.Options) {
			o.JWT.ActiveKID = "k1"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := baseOpts()
			tc.mut(&opts)
			if _, err := New(opts); !errors.Is(err, auth.ErrInvalidOptions) {
				t.Errorf("New() error = %v, want ErrInvalidOptions", err)
			}
		})
	}
}
