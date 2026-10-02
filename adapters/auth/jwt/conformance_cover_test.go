package jwt

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/auth/authtest"
)

const conformanceSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

// TestJWTConformance proves the JWT adapter honors the auth.Auth contract
// via the shared conformance kit. Each subtest gets a fresh adapter with
// its own in-memory revocation store (no network).
func TestJWTConformance(t *testing.T) {
	t.Parallel()

	authtest.Conformance(t, func(t *testing.T) auth.Auth {
		t.Helper()

		a, err := New(auth.Options{JWT: auth.JWTOptions{
			Secret:   conformanceSecret,
			Issuer:   "conformance-iss",
			Audience: "conformance-aud",
			MaxTTL:   time.Hour,
		}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
