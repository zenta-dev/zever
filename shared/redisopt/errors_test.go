package redisopt

import (
	"errors"
	"strings"
	"testing"
)

// TestInvalidAddressError_namesTheAddress proves the message names the
// address that failed to parse: the developer needs to know which of several
// configured addresses is wrong.
func TestInvalidAddressError_namesTheAddress(t *testing.T) {
	t.Parallel()

	err := &InvalidAddressError{Addr: "redis://h:6379", Err: errors.New("missing host")}

	want := `redis: invalid address "redis://h:6379": missing host`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestInvalidAddressError_redactsCredentials proves a password embedded in a
// rejected address never reaches the error text.
func TestInvalidAddressError_redactsCredentials(t *testing.T) {
	t.Parallel()

	const secret = "s3cret"

	err := &InvalidAddressError{Addr: "redis://bob:" + secret + "@h:6379"}

	if strings.Contains(err.Error(), secret) {
		t.Errorf("Error() = %q, leaks the password", err.Error())
	}

	if !strings.Contains(err.Error(), "redis://bob:xxxxx@h:6379") {
		t.Errorf("Error() = %q, want the masked address", err.Error())
	}
}

// TestPlaintextRejectedError_namesTheFix proves the message says which
// address was refused in plaintext and how to turn TLS on.
func TestPlaintextRejectedError_namesTheFix(t *testing.T) {
	t.Parallel()

	err := &PlaintextRejectedError{Addr: "h:6379"}

	want := `redis: plaintext rejected "h:6379": set tls, or use a rediss:// address, to connect over TLS`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestPlaintextRejectedError_redactsCredentials proves a password embedded in
// the refused address never reaches the error text.
func TestPlaintextRejectedError_redactsCredentials(t *testing.T) {
	t.Parallel()

	const secret = "s3cret"

	err := &PlaintextRejectedError{Addr: "redis://bob:" + secret + "@h:6379"}

	if strings.Contains(err.Error(), secret) {
		t.Errorf("Error() = %q, leaks the password", err.Error())
	}
}
