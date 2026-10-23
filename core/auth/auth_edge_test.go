package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// nilCtx is a nil context used to prove the battery never dereferences ctx
// before the adapter boundary.
var nilCtx context.Context

func TestEdgeOptionsValidate_negativeLeeway(t *testing.T) {
	t.Parallel()

	err := Options{JWT: JWTOptions{Leeway: -time.Second}}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "jwt_leeway") {
		t.Fatalf("err %q missing jwt_leeway reason", err.Error())
	}
	var ioe InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestEdgeOptionsValidate_zeroMaxTTLValid(t *testing.T) {
	t.Parallel()

	// Zero MaxTTL means no cap; zero Leeway means strict validation.
	if err := (Options{JWT: JWTOptions{MaxTTL: 0, Leeway: 0}}).Validate(); err != nil {
		t.Fatalf("Validate err = %v, want nil", err)
	}
}

func TestEdgeOptionsValidate_oidcIssuerQueryFragmentUserinfo(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://idp.example.com/auth?foo=bar",
		"https://idp.example.com/auth#frag",
		"https://user:pass@idp.example.com",
	}
	for _, issuer := range cases {
		opts := Options{OIDC: OIDCOptions{Issuer: issuer, ClientID: "cid"}}
		if err := opts.Validate(); err != nil {
			t.Errorf("Validate(%q) err = %v, want nil", issuer, err)
		}
	}
}

func TestEdgeClaimsClone_emptyCustom(t *testing.T) {
	t.Parallel()

	cp := Claims{}.Clone()
	if cp.Custom != nil {
		t.Fatalf("Clone Custom = %v, want nil", cp.Custom)
	}
	if cp.Subject != "" {
		t.Fatalf("Clone Subject = %q, want empty", cp.Subject)
	}
}

func TestEdgeCloneValue_nestedMapStringMap(t *testing.T) {
	t.Parallel()

	in := map[string]any{"m": map[string]string{"k": "v"}}
	out := CloneValue(in)
	inner := mustStrStrMap(t, mustStrMap(t, out)["m"])
	inner["k"] = "mutant"
	if mustStrStrMap(t, in["m"])["k"] != "v" {
		t.Fatal("CloneValue shares nested map[string]string")
	}
}

func TestEdgeCloneValue_bytesInsideSlice(t *testing.T) {
	t.Parallel()

	in := []any{[]byte{1, 2}}
	out := CloneValue(in)
	mustBytes(t, mustAnySlice(t, out)[0])[0] = 9
	if mustBytes(t, in[0])[0] != 1 {
		t.Fatal("CloneValue shares []byte inside []any")
	}
}

func TestEdgeCloneValue_nilContainers(t *testing.T) {
	t.Parallel()

	// Nil containers clone to empty non-nil containers.
	if got := CloneValue(map[string]any(nil)); got == nil {
		t.Fatal("CloneValue(nil map) = nil, want empty map")
	}
	if got := CloneValue([]any(nil)); got == nil {
		t.Fatal("CloneValue(nil slice) = nil, want empty slice")
	}
}

func TestEdgeToken_zero(t *testing.T) {
	t.Parallel()

	var tok Token
	if tok.Value != "" {
		t.Fatalf("zero Token.Value = %q, want empty", tok.Value)
	}
	if !tok.ExpiresAt.IsZero() {
		t.Fatalf("zero Token.ExpiresAt = %v, want zero", tok.ExpiresAt)
	}
}
