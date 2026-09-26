package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestOptions_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("zero Options Validate err = %v", err)
	}
}

func TestOptions_negative_ttl_fails(t *testing.T) {
	t.Parallel()
	err := Options{TTL: -1}.Validate()
	if err == nil {
		t.Fatal("negative TTL expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err type = %T, want *InvalidOptionsError", err)
	}
}

func TestOptions_bad_addr_scheme_fails(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"redis://localhost:6379", "http://h:6379", "rediss://h:6379"} {
		err := Options{Redis: RedisOptions{Addr: addr}}.Validate()
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("addr %q err = %v, want ErrInvalidOptions", addr, err)
		}
	}
}

func TestOptions_bad_addr_shape_fails(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"localhost", ":6379", "localhost:", "h:0", "h:99999", "h:notaport", "h:1/a", "h:1?x", "h:1#y"} {
		err := Options{Redis: RedisOptions{Addr: addr}}.Validate()
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("addr %q err = %v, want ErrInvalidOptions", addr, err)
		}
	}
}

func TestOptions_good_addr_success(t *testing.T) {
	t.Parallel()
	err := Options{Redis: RedisOptions{Addr: "localhost:6379"}}.Validate()
	if err != nil {
		t.Fatalf("good addr Validate err = %v", err)
	}
}

func TestOptions_oversize_prefix_fails(t *testing.T) {
	t.Parallel()
	err := Options{Redis: RedisOptions{Prefix: strings.Repeat("a", 65)}}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("oversize prefix err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_bad_prefix_chars_fails(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"has space", "slash/x", "colon:x"} {
		err := Options{Redis: RedisOptions{Prefix: p}}.Validate()
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("prefix %q err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_ttl_default_applied(t *testing.T) {
	t.Parallel()
	if got := (Options{}).ttl(); got != DefaultTTL {
		t.Fatalf("zero ttl() = %v, want %v", got, DefaultTTL)
	}
}
