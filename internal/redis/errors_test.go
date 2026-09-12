package redis

import (
	"errors"
	"testing"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()
	if ErrInvalidAddress.Error() != "redis: invalid address" {
		t.Fatalf("ErrInvalidAddress = %q", ErrInvalidAddress.Error())
	}
	if ErrParseAddress.Error() != "redis: parse address failed" {
		t.Fatalf("ErrParseAddress = %q", ErrParseAddress.Error())
	}
	if ErrCloseClient.Error() != "redis: close client failed" {
		t.Fatalf("ErrCloseClient = %q", ErrCloseClient.Error())
	}
}

func TestToRedisOptionsMissingHost(t *testing.T) {
	t.Parallel()
	_, err := Options{Addr: "redis://"}.toRedisOptions()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v, want ErrInvalidAddress", err)
	}
	var addrErr *InvalidAddressError
	if !errors.As(err, &addrErr) {
		t.Fatalf("err = %T %v, want *InvalidAddressError", err, err)
	}
}

func TestToRedisOptionsBadURL(t *testing.T) {
	t.Parallel()
	_, err := Options{Addr: "redis://%zz"}.toRedisOptions()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v, want ErrInvalidAddress", err)
	}
	if !errors.Is(err, ErrParseAddress) {
		t.Fatalf("err = %v, want ErrParseAddress", err)
	}
}

func TestToRedisOptionsValid(t *testing.T) {
	t.Parallel()
	opt, err := Options{Addr: "localhost:6379"}.toRedisOptions()
	if err != nil {
		t.Fatalf("toRedisOptions() err = %v", err)
	}
	if opt.Addr != "localhost:6379" {
		t.Fatalf("opt.Addr = %q", opt.Addr)
	}
}

func TestInvalidAddressError_messageFormats(t *testing.T) {
	t.Parallel()

	t.Run("with inner error", func(t *testing.T) {
		t.Parallel()

		err := &InvalidAddressError{Addr: "redis://", Err: errors.New("missing host")}
		want := `redis: invalid address "redis://": missing host`
		if got := err.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without inner error", func(t *testing.T) {
		t.Parallel()

		err := &InvalidAddressError{Addr: "redis://"}
		want := `redis: invalid address "redis://"`
		if got := err.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}

func TestInvalidAddressError_unwrapMatchesBoth(t *testing.T) {
	t.Parallel()

	inner := errors.New("missing host")
	err := &InvalidAddressError{Addr: "redis://", Err: inner}

	if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("errors.Is(err, ErrInvalidAddress) = false (err = %v)", err)
	}

	if !errors.Is(err, inner) {
		t.Errorf("errors.Is(err, inner) = false (err = %v)", err)
	}

	bare := &InvalidAddressError{Addr: "redis://"}
	if !errors.Is(bare, ErrInvalidAddress) {
		t.Errorf("errors.Is(bare, ErrInvalidAddress) = false (err = %v)", bare)
	}
}

func TestNew_invalidAddr_wrapsInvalidAddress(t *testing.T) {
	if _, err := New(Options{Addr: "redis://"}); err == nil {
		t.Fatal("New() = nil, want wrapped InvalidAddressError")
	} else if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("errors.Is(err, ErrInvalidAddress) = false (err = %v)", err)
	}

	if err := Close(); err != nil {
		t.Errorf("Close() cleanup error = %v, want nil", err)
	}
}
