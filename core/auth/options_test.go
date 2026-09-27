package auth

import (
	"errors"
	"testing"
	"time"
)

func TestOptions_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("zero Options Validate err = %v", err)
	}
}

func TestOptions_negative_jwt_maxttl_fails(t *testing.T) {
	t.Parallel()
	err := Options{JWT: JWTOptions{MaxTTL: -time.Second}}.Validate()
	if err == nil {
		t.Fatal("negative JWT MaxTTL expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err type = %T, want *InvalidOptionsError", err)
	}
}

func TestOptions_negative_oidc_timeout_fails(t *testing.T) {
	t.Parallel()
	err := Options{OIDC: OIDCOptions{Timeout: -time.Second}}.Validate()
	if err == nil {
		t.Fatal("negative OIDC Timeout expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err type = %T, want *InvalidOptionsError", err)
	}
}

func TestOptions_positive_valid(t *testing.T) {
	t.Parallel()
	opts := Options{
		JWT:  JWTOptions{MaxTTL: time.Hour},
		OIDC: OIDCOptions{Issuer: "https://example.com", ClientID: "cid", Timeout: time.Second},
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("positive Options Validate err = %v", err)
	}
}
