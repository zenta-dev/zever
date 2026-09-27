package auth

import (
	"errors"
	"testing"
)

func TestAdapter_String_jwt(t *testing.T) {
	t.Parallel()
	if got := JWT.String(); got != "jwt" {
		t.Fatalf("JWT.String() = %q, want %q", got, "jwt")
	}
}

func TestAdapter_String_session(t *testing.T) {
	t.Parallel()
	if got := Session.String(); got != "session" {
		t.Fatalf("Session.String() = %q, want %q", got, "session")
	}
}

func TestAdapter_String_oidc(t *testing.T) {
	t.Parallel()
	if got := OIDC.String(); got != "oidc" {
		t.Fatalf("OIDC.String() = %q, want %q", got, "oidc")
	}
}

func TestAdapter_String_unknown(t *testing.T) {
	t.Parallel()
	if got := Adapter("").String(); got != "unknown" {
		t.Fatalf("Adapter(empty).String() = %q, want %q", got, "unknown")
	}
}

func TestParseAdapter_jwt_success(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("jwt")
	if err != nil {
		t.Fatalf("ParseAdapter(jwt) err = %v", err)
	}
	if a != JWT {
		t.Fatalf("ParseAdapter(jwt) = %v, want JWT", a)
	}
}

func TestParseAdapter_session_success(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("session")
	if err != nil {
		t.Fatalf("ParseAdapter(session) err = %v", err)
	}
	if a != Session {
		t.Fatalf("ParseAdapter(session) = %v, want Session", a)
	}
}

func TestParseAdapter_oidc_success(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("oidc")
	if err != nil {
		t.Fatalf("ParseAdapter(oidc) err = %v", err)
	}
	if a != OIDC {
		t.Fatalf("ParseAdapter(oidc) = %v, want OIDC", a)
	}
}

func TestParseAdapter_uppercase_rejected(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"JWT", "Session", "OIDC"} {
		a, err := ParseAdapter(s)
		if err != nil {
			t.Fatalf("ParseAdapter(%q) err = %v, want nil (open adapter)", s, err)
		}
		if a != Adapter(s) {
			t.Fatalf("ParseAdapter(%q) adapter = %v, want %v", s, a, Adapter(s))
		}
	}
}

func TestParseAdapter_unknown_fails_zero(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("redis")
	if err != nil {
		t.Fatalf("ParseAdapter(redis) err = %v, want nil (open adapter)", err)
	}
	if a != Adapter("redis") {
		t.Fatalf("ParseAdapter(redis) adapter = %v, want %v", a, Adapter("redis"))
	}
}

func TestParseAdapter_empty_fails_zero(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("")
	if err == nil {
		t.Fatal("ParseAdapter empty expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter empty err = %v, want ErrInvalidAdapter", err)
	}
	if a != Adapter("") {
		t.Fatalf("ParseAdapter empty adapter = %v, want empty zero value", a)
	}
}
