package jwt

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
)

// Seed tokens below are deterministic (fixed claims, fixed keys) so the fuzz
// corpus stays stable: they were minted with rotationSecretB under kid "k2"
// (wrong-kid uses testSecret under kid "wrong-kid", none uses no signature)
// with issuer "test-iss" and audience "test-aud", matching rotationOpts.
const (
	// fuzzValidSeed is a well-formed token that verifies under rotationOpts.
	fuzzValidSeed = "eyJhbGciOiJIUzI1NiIsImtpZCI6ImsyIiwidHlwIjoiSldUIn0.eyJhdWQiOiJ0ZXN0LWF1ZCIsImV4cCI6NDEwMjQ0NDgwMCwiaWF0IjoxNzAwMDAwMDAwLCJpc3MiOiJ0ZXN0LWlzcyIsImp0aSI6InNlZWQtanRpLTEiLCJzdWIiOiJzZWVkLXVzZXIifQ.Ks7xb3bSVNCh_op56nhQlmPXNCcEi9Bg-dcD5tXG2eE"
	// fuzzNoneSeed carries an alg=none header and must be rejected.
	fuzzNoneSeed = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJhdWQiOiJ0ZXN0LWF1ZCIsImV4cCI6NDEwMjQ0NDgwMCwiaWF0IjoxNzAwMDAwMDAwLCJpc3MiOiJ0ZXN0LWlzcyIsInN1YiI6Im1hbGxvcnkifQ."
	// fuzzWrongKIDSeed is correctly signed but names an unknown kid.
	fuzzWrongKIDSeed = "eyJhbGciOiJIUzI1NiIsImtpZCI6Indyb25nLWtpZCIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ0ZXN0LWF1ZCIsImV4cCI6NDEwMjQ0NDgwMCwiaWF0IjoxNzAwMDAwMDAwLCJpc3MiOiJ0ZXN0LWlzcyIsImp0aSI6InNlZWQtanRpLTEiLCJzdWIiOiJzZWVkLXVzZXIifQ.J_0yvtdQ_R4SiDnHdTEHGfpLBSrM6cM3spLsyjPuprs"
	// fuzzExpiredSeed is correctly signed but long past its exp claim.
	fuzzExpiredSeed = "eyJhbGciOiJIUzI1NiIsImtpZCI6ImsyIiwidHlwIjoiSldUIn0.eyJhdWQiOiJ0ZXN0LWF1ZCIsImV4cCI6MTAwMCwiaWF0Ijo1MDAsImlzcyI6InRlc3QtaXNzIiwianRpIjoic2VlZC1qdGktZXhwaXJlZCIsInN1YiI6InNlZWQtdXNlciJ9.JOLvN9Z3bSU6FXeW1NCWfzSHCVytxhiIobtJYkorJG8"
	// fuzzTruncatedSeed is the valid token cut mid-payload.
	fuzzTruncatedSeed = "eyJhbGciOiJIUzI1NiIsImtpZCI6ImsyIiwidHlwIjoiSldUIn0.eyJhdWQiOiJ0ZXN0LWF1ZCIsImV4cCI6NDEwMjQ0NDgwMCwiaWF0IjoxNzAwMDAwMDA"
)

// fuzzAdapter builds an isolated rotation adapter for fuzz iterations: kid
// routing, legacy fallback, and alg rejection all resolve against the
// rotation keys, while the per-iteration fake revocation store keeps inputs
// from sharing revocation state.
func fuzzAdapter(tb testing.TB) auth.Auth {
	tb.Helper()
	opts := rotationOpts()
	opts.JWT.RevocationStore = newFakeRevocationStore()
	return freshAdapter(tb, opts)
}

// FuzzVerify proves Verify never panics and fails closed on hostile tokens:
// arbitrary strings, truncations, non-base64 segments, alg-confusion
// (none/RS256) headers, unknown kids, and missing or expired exp claims all
// error, while tokens minted by the adapter's own Issue still verify.
func FuzzVerify(f *testing.F) {
	f.Add(fuzzValidSeed)
	f.Add(fuzzNoneSeed)
	f.Add(fuzzWrongKIDSeed)
	f.Add(fuzzExpiredSeed)
	f.Add(fuzzTruncatedSeed)
	f.Add("")
	f.Add("!!!not-base64!!!")

	f.Fuzz(func(t *testing.T, token string) {
		t.Parallel()

		a := fuzzAdapter(t)
		ctx := t.Context()

		// Round-trip sanity: tokens minted by Issue still verify.
		good, err := a.Issue(ctx, "fuzz-subject", nil, time.Minute)
		if err != nil {
			t.Fatalf("Issue() error = %v", err)
		}
		if _, verifyErr := a.Verify(ctx, good.Value); verifyErr != nil {
			t.Fatalf("Verify(Issue()) error = %v, want nil", verifyErr)
		}

		// Arbitrary input must fail closed or, if it verifies, carry
		// well-formed claims; errors stay inside the auth taxonomy.
		claims, err := a.Verify(ctx, token)
		if err == nil {
			if claims.Subject == "" {
				t.Fatalf("Verify(%q) succeeded with empty subject", token)
			}
			if claims.ExpiresAt.IsZero() {
				t.Fatalf("Verify(%q) succeeded with zero expiry", token)
			}
		} else if !errors.Is(err, auth.ErrInvalidToken) &&
			!errors.Is(err, auth.ErrTokenExpired) &&
			!errors.Is(err, auth.ErrTokenRevoked) {
			t.Fatalf("Verify(%q) error = %v, want auth taxonomy", token, err)
		}

		// Attacker variants spliced from the valid token must fail closed.
		parts := strings.Split(good.Value, ".")
		if len(parts) != 3 {
			t.Fatalf("Issue() token has %d segments, want 3", len(parts))
		}
		mustFail := func(name, raw string) {
			t.Helper()
			if _, verifyErr := a.Verify(ctx, raw); !errors.Is(verifyErr, auth.ErrInvalidToken) {
				t.Fatalf("Verify(%s) error = %v, want ErrInvalidToken", name, verifyErr)
			}
		}
		noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
		mustFail("alg=none", noneHeader+"."+parts[1]+".")
		rsHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
		mustFail("alg=RS256", rsHeader+"."+parts[1]+"."+parts[2])
		badKIDHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","kid":"attacker-kid","typ":"JWT"}`))
		mustFail("unknown kid", badKIDHeader+"."+parts[1]+"."+parts[2])
		mustFail("missing segment", parts[0]+"."+parts[1])
		mustFail("empty signature", parts[0]+"."+parts[1]+".")
		mustFail("non-base64", "!!!."+parts[1]+"."+parts[2])
		mustFail("empty", "")
		noExp := signWithKID(t, "k2", rotationSecretB, jwtv5.MapClaims{
			"sub": "mallory",
			"iss": "test-iss",
			"aud": "test-aud",
		})
		mustFail("missing exp", noExp)

		// Truncation at a fuzzer-chosen cut fails unless the cut kept the
		// whole token.
		cut := len(token) % (len(good.Value) + 1)
		if trunc := good.Value[:cut]; trunc != good.Value {
			mustFail("truncated", trunc)
		}

		// An expired token signed with the right key fails as expired.
		past := signWithKID(t, "k2", rotationSecretB, jwtv5.MapClaims{
			"sub": "mallory",
			"iss": "test-iss",
			"aud": "test-aud",
			"exp": time.Now().Add(-time.Hour).Unix(),
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
		})
		if _, verifyErr := a.Verify(ctx, past); !errors.Is(verifyErr, auth.ErrTokenExpired) {
			t.Fatalf("Verify(expired) error = %v, want ErrTokenExpired", verifyErr)
		}
	})
}
