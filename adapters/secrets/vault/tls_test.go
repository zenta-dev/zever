package vault

import (
	"strings"
	"testing"
)

// TestValidate_HTTPSAcceptedByDefault pins the secure default: an https
// Addr needs no opt-in.
func TestValidate_HTTPSAcceptedByDefault(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "https://vault.example.com:8200", Token: "t", Mount: "secret"}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// TestValidate_HTTPRejectedWithoutAllowInsecure pins the P2-11a fix: a
// plain-http Addr (token would travel in cleartext) fails closed unless
// the operator explicitly opts into insecure transport.
func TestValidate_HTTPRejectedWithoutAllowInsecure(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "secret"}
	err := opts.Validate()
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("Validate() = %v, want https-required error", err)
	}
}

// TestValidate_HTTPAllowedWithAllowInsecure pins the explicit opt-out:
// test setups (httptest is plain http) keep working by setting the flag.
func TestValidate_HTTPAllowedWithAllowInsecure(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "http://127.0.0.1:8200", Token: "t", Mount: "secret", AllowInsecure: true}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// TestValidate_NonHTTPSSchemeRejected ensures allow_insecure does not open
// the door to non-http schemes.
func TestValidate_NonHTTPSSchemeRejected(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"ftp://vault.example.com/secret", "gopher://vault.example.com/secret"} {
		opts := Options{Addr: addr, Token: "t", Mount: "secret", AllowInsecure: true}
		err := opts.Validate()
		if err == nil || !strings.Contains(err.Error(), "scheme") {
			t.Errorf("Validate(%q) = %v, want scheme error", addr, err)
		}
	}
}

// TestNew_HTTPWithoutAllowInsecure_FailsClosed proves New enforces the
// same rule (Validate is not optional at construction).
func TestNew_HTTPWithoutAllowInsecure_FailsClosed(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{Addr: "http://127.0.0.1:1", Token: "t", Mount: "secret"}); err == nil {
		t.Fatal("New(http without allow_insecure) = nil, want error")
	}
}
