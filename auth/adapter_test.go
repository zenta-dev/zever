package auth

import (
	"errors"
	"strings"
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
	if got := Adapter(99).String(); got != "unknown" {
		t.Fatalf("Adapter(99).String() = %q, want %q", got, "unknown")
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
		if err == nil {
			t.Fatalf("ParseAdapter(%q) expected error, got nil", s)
		}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Fatalf("ParseAdapter(%q) err = %v, want ErrInvalidAdapter", s, err)
		}
		if a != JWT {
			t.Fatalf("ParseAdapter(%q) adapter = %v, want JWT zero value", s, a)
		}
	}
}

func TestParseAdapter_unknown_fails_zero(t *testing.T) {
	t.Parallel()
	a, err := ParseAdapter("redis")
	if err == nil {
		t.Fatal("ParseAdapter(redis) expected error, got nil")
	}
	var iae *InvalidAdapterError
	if !errors.As(err, &iae) {
		t.Fatalf("ParseAdapter(redis) err type = %T, want *InvalidAdapterError", err)
	}
	if iae.Adapter != "redis" {
		t.Fatalf("InvalidAdapterError.Adapter = %q, want %q", iae.Adapter, "redis")
	}
	if a != JWT {
		t.Fatalf("ParseAdapter(redis) adapter = %v, want JWT zero value", a)
	}
	if !strings.Contains(err.Error(), "redis") {
		t.Fatalf("error message should carry adapter name, got %q", err.Error())
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
	if a != JWT {
		t.Fatalf("ParseAdapter empty adapter = %v, want JWT zero value", a)
	}
}
