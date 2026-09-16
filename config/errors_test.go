package config

import (
	"errors"
	"testing"
)

func TestErrUnknownService_message(t *testing.T) {
	t.Parallel()

	if got, want := ErrUnknownService.Error(), "config: unknown service"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrUnknownField_message(t *testing.T) {
	t.Parallel()

	if got, want := ErrUnknownField.Error(), "config: unknown field"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrUnsupportedFormat_message(t *testing.T) {
	t.Parallel()

	if got, want := ErrUnsupportedFormat.Error(), "config: unsupported format"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrInvalidOptions_message(t *testing.T) {
	t.Parallel()

	if got, want := ErrInvalidOptions.Error(), "config: invalid options"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrDecode_message(t *testing.T) {
	t.Parallel()

	if got, want := ErrDecode.Error(), "config: decode"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnknownServiceError_errorAndUnwrap(t *testing.T) {
	t.Parallel()

	err := &UnknownServiceError{Service: "nope"}

	if got, want := err.Error(), `config: unknown service "nope"`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, ErrUnknownService) {
		t.Errorf("errors.Is(%v, ErrUnknownService) = false, want true", err)
	}

	var target *UnknownServiceError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}

	if target.Service != "nope" {
		t.Errorf("Service = %q, want %q", target.Service, "nope")
	}
}

func TestUnknownFieldError_errorAndUnwrap(t *testing.T) {
	t.Parallel()

	err := &UnknownFieldError{Service: "db", Field: "bogus"}

	if got, want := err.Error(), `config: unknown field "bogus" for service "db"`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, ErrUnknownField) {
		t.Errorf("errors.Is(%v, ErrUnknownField) = false, want true", err)
	}

	var target *UnknownFieldError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}

	if target.Service != "db" || target.Field != "bogus" {
		t.Errorf("got {%q %q}, want {db bogus}", target.Service, target.Field)
	}
}

func TestDecodeError_errorAndUnwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")
	err := &DecodeError{Service: "db", Err: inner}

	if got, want := err.Error(), "config: decode db: boom"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, inner) {
		t.Errorf("errors.Is did not match inner error %v", inner)
	}

	var target *DecodeError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}

	if target.Service != "db" {
		t.Errorf("Service = %q, want %q", target.Service, "db")
	}
}

func TestDecodeError_unwrapChainPreservesSentinel(t *testing.T) {
	t.Parallel()

	wrapped := &DecodeError{Service: "db", Err: ErrDecode}

	if !errors.Is(wrapped, ErrDecode) {
		t.Errorf("errors.Is(%v, ErrDecode) = false, want true", wrapped)
	}
}

func TestInvalidOptionsError_errorAndUnwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidOptionsError{Service: "cache", Reason: "addr required"}

	if got, want := err.Error(), "config: invalid options cache: addr required"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("errors.Is(%v, ErrInvalidOptions) = false, want true", err)
	}

	var target *InvalidOptionsError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}

	if target.Service != "cache" || target.Reason != "addr required" {
		t.Errorf("got {%q %q}, want {cache addr required}", target.Service, target.Reason)
	}
}
