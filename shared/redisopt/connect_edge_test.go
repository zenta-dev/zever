package redisopt

import (
	"errors"
	"strings"
	"testing"
)

// TestValidatePrefix_byteLength verifies the limit is measured in bytes.
func TestValidatePrefix_byteLength(t *testing.T) {
	t.Parallel()

	// 22 three-byte runes = 66 bytes > 64, though only 22 characters.
	tooLong := strings.Repeat("世", 22)
	if err := ValidatePrefix(tooLong); err == nil {
		t.Errorf("ValidatePrefix(%d-byte prefix) = nil, want length error", len(tooLong))
	}
}

// TestInvalidAddressError_unwrapMatchesSentinel verifies errors.Is resolves
// to ErrInvalidAddress through Unwrap.
func TestInvalidAddressError_unwrapMatchesSentinel(t *testing.T) {
	t.Parallel()

	e := &InvalidAddressError{Addr: "bad", Err: errors.New("boom")}
	if !errors.Is(e, ErrInvalidAddress) {
		t.Error("errors.Is(e, ErrInvalidAddress) = false")
	}
}

// TestInvalidAddressError_unwrapExposedForAs verifies the underlying error
// is reachable via Unwrap for errors.Is inspection.
func TestInvalidAddressError_unwrapExposedForAs(t *testing.T) {
	t.Parallel()

	underlying := errBoom
	e := &InvalidAddressError{Addr: "bad", Err: underlying}

	if !errors.Is(e, underlying) {
		t.Error("errors.Is(e, underlying) = false")
	}
}

// errBoom is a sentinel for unwrap-traversal checks.
var errBoom = errors.New("boom")

// TestPlaintextRejectedError_unwrapMatchesSentinel verifies errors.Is
// resolves to ErrPlaintextRejected through Unwrap.
func TestPlaintextRejectedError_unwrapMatchesSentinel(t *testing.T) {
	t.Parallel()

	e := &PlaintextRejectedError{Addr: "127.0.0.1:6379"}
	if !errors.Is(e, ErrPlaintextRejected) {
		t.Error("errors.Is(e, ErrPlaintextRejected) = false")
	}
}
