package jwt

import (
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
)

func TestIssue_UnserializableClaimsFail(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, baseOpts())
	_, err := a.Issue(t.Context(), "alice", map[string]any{"bad": func() {}}, time.Minute)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Issue(unserializable) error = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_EmptySubjectRejected(t *testing.T) {
	t.Parallel()
	a := freshAdapter(t, baseOpts())
	raw := signManual(t, jwtv5.MapClaims{
		"iss": "test-iss",
		"aud": "test-aud",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	_, err := a.Verify(t.Context(), raw)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(missing sub) error = %v, want ErrInvalidToken", err)
	}
}
