package vault

import (
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/secrets/secretstest"
)

// TestVaultConformance proves the vault adapter honors the secrets.Secrets
// contract via the shared conformance kit. Each subtest gets a fresh
// httptest-backed instance (loopback only, no external network) so cases
// stay isolated.
func TestVaultConformance(t *testing.T) {
	secretstest.Conformance(t, func(t *testing.T) secrets.Secrets {
		t.Helper()

		srv := mustStubServer(t, "test-token", "secret")
		s := mustNew(t, Options{Addr: srv.URL, Token: "test-token", Mount: "secret"})

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}
