package authz

import (
	"testing"

	"github.com/zenta-dev/zever/core/auth"
)

func TestClaimsContextIsolation(t *testing.T) {
	t.Parallel()

	want := auth.Claims{Subject: "u1", Custom: map[string]any{"k": "v"}}
	ctx := withClaims(t.Context(), want)

	// Mutating the source after storing must not affect the context.
	want.Custom["k"] = "MUT"

	got, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext() ok = false, want true")
	}
	if got.Custom["k"] != "v" {
		t.Fatalf("stored claims mutated via source map: %v", got.Custom)
	}

	// Mutating a lookup result must not affect later lookups.
	got.Custom["k"] = "MUT2"
	again, ok := ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext() ok = false, want true")
	}
	if again.Custom["k"] != "v" {
		t.Fatalf("stored claims mutated via lookup result: %v", again.Custom)
	}
}
