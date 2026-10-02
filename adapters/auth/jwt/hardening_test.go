package jwt

import (
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
)

func TestNew_RejectsNegativeLeeway(t *testing.T) {
	t.Parallel()
	opts := baseOpts()
	opts.JWT.Leeway = -time.Second
	_, err := New(opts)
	if err == nil {
		t.Fatal("New() with negative Leeway succeeded, want error")
	}
	if !errors.Is(err, auth.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
	}
}

// TestVerify_LeewayBoundary proves leeway only forgives skew inside its
// window: a token 2s past expiry verifies with a 10s leeway but stays
// expired by default, and a token 20s past expiry stays expired even with
// the leeway.
func TestVerify_LeewayBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expiredAgo time.Duration
		leeway     time.Duration
		wantErr    error
	}{
		{name: "default strict still expired", expiredAgo: 2 * time.Second, leeway: 0, wantErr: auth.ErrTokenExpired},
		{name: "leeway forgives recent expiry", expiredAgo: 2 * time.Second, leeway: 10 * time.Second, wantErr: nil},
		{name: "leeway does not forgive old expiry", expiredAgo: 20 * time.Second, leeway: 10 * time.Second, wantErr: auth.ErrTokenExpired},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := baseOpts()
			opts.JWT.Leeway = tc.leeway
			a := freshAdapter(t, opts)
			raw := signManual(t, jwtv5.MapClaims{
				"sub": "alice",
				"iss": "test-iss",
				"aud": "test-aud",
				"exp": time.Now().Add(-tc.expiredAgo).Unix(),
				"iat": time.Now().Add(-time.Hour).Unix(),
			})
			got, err := a.Verify(t.Context(), raw)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Verify() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v, want nil", err)
			}
			if got.Subject != "alice" {
				t.Fatalf("Verify() subject = %q, want alice", got.Subject)
			}
		})
	}
}

// TestClose_ThenIssueFails proves Close zeroes the HMAC secret and fails
// later operations closed instead of signing keyless.
func TestClose_ThenIssueFails(t *testing.T) {
	t.Parallel()
	a, err := New(baseOpts())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := t.Context()
	tok, err := a.Issue(ctx, "alice", nil, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := a.Verify(ctx, tok.Value); err != nil {
		t.Fatalf("Verify() before Close error = %v, want nil", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	inner, ok := a.(*adapter)
	if !ok {
		t.Fatalf("New() type = %T, want *adapter", a)
	}
	for i, b := range inner.secret {
		if b != 0 {
			t.Fatalf("secret[%d] = %#02x after Close, want zeroed", i, b)
		}
	}

	if _, err := a.Issue(ctx, "alice", nil, time.Minute); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Issue() after Close error = %v, want ErrInvalidToken", err)
	}
	if _, err := a.Verify(ctx, tok.Value); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify() after Close error = %v, want ErrInvalidToken", err)
	}
	if err := a.Revoke(ctx, tok.Value); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Revoke() after Close error = %v, want ErrInvalidToken", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}
}
