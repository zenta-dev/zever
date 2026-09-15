package authz

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/auth"
)

func TestClaimsRoundtrip(t *testing.T) {
	t.Parallel()

	want := auth.Claims{Subject: "u1", Custom: map[string]any{"k": "v"}}
	ctx := withClaims(context.Background(), want)
	got, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext() ok = false, want true")
	}
	if got.Subject != "u1" {
		t.Fatalf("Subject = %q, want u1", got.Subject)
	}
}

func TestClaimsMissing(t *testing.T) {
	t.Parallel()

	if _, ok := ClaimsFromContext(context.Background()); ok {
		t.Fatal("ClaimsFromContext() ok = true, want false")
	}
}

func TestClaimsWrongType(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), claimsKey{}, "not-claims")
	if _, ok := ClaimsFromContext(ctx); ok {
		t.Fatal("ClaimsFromContext() ok = true for wrong type, want false")
	}
}
