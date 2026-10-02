package oidc_test

import (
	"testing"
)

// TestOIDCConformance is verify-only by necessity: New runs OIDC discovery
// against the issuer over the network, so no offline conformance factory
// can build the adapter. Issue/Revoke always return ErrNotSupported (the
// IdP owns the token lifecycle) and Verify is covered against recorded
// tokens in oidc_test.go. This stub documents the exemption; it runs no
// kit cases.
func TestOIDCConformance(t *testing.T) {
	t.Parallel()

	t.Skip("oidc conformance is verify-only: adapter construction requires live provider discovery (network)")
}
