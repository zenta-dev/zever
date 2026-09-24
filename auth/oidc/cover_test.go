package oidc_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth"
)

func TestCoverVerifyClaimsExtractFails(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	// "email" as a number passes signature/issuer/audience/expiry but
	// fails extraction into the string-typed claim struct.
	raw := mintToken(t, idp.key, idp.kid, idp.srv.URL, "test-client",
		time.Now().Add(time.Hour), map[string]any{"email": 123})

	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify err = %v, want ErrInvalidToken", err)
	}
}
