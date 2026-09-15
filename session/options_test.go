package session

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestOptions_ttl_default_applied(t *testing.T) {
	t.Parallel()
	if got := (Options{}).ttl(); got != DefaultTTL {
		t.Fatalf("zero ttl() = %v, want %v", got, DefaultTTL)
	}
}

func TestOptions_redis_valid(t *testing.T) {
	t.Parallel()
	opts := Options{Redis: RedisOptions{Addr: "localhost:6379", Prefix: "sess"}}
	if err := opts.Validate(); err != nil {
		t.Fatalf("redis Options Validate err = %v", err)
	}
}

func TestOptions_redis_prefix_token_chars_valid(t *testing.T) {
	t.Parallel()

	opts := Options{Redis: RedisOptions{Prefix: "a!b_c.d"}}
	if err := opts.Validate(); err != nil {
		t.Fatalf("token-char prefix Validate err = %v", err)
	}
}

func TestOptions_redis_prefix_token_special_char_valid(t *testing.T) {
	t.Parallel()

	opts := Options{Redis: RedisOptions{Prefix: "sess!_x"}}
	if err := opts.Validate(); err != nil {
		t.Fatalf("prefix with token special chars Validate err = %v", err)
	}
}

func TestOptions_redis_prefix_disallowed_classes_fail(t *testing.T) {
	t.Parallel()

	for _, p := range []string{
		"has space",
		"line\nbreak",
		"nul\x00byte",
		"slash/prefix",
		"colon:prefix",
	} {
		err := Options{Redis: RedisOptions{Prefix: p}}.Validate()
		if err == nil {
			t.Fatalf("prefix %q expected error, got nil", p)
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("prefix %q err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_redis_bad_addr_fails(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"https://h:6379", "h:6379/x", "h", ":0", "h:99999", ":6379"} {
		err := Options{Redis: RedisOptions{Addr: addr}}.Validate()
		if err == nil {
			t.Fatalf("addr %q expected error, got nil", addr)
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("addr %q err = %v, want ErrInvalidOptions", addr, err)
		}
	}
}

func TestOptions_redis_bad_prefix_fails(t *testing.T) {
	t.Parallel()
	for _, p := range []string{strings.Repeat("p", 65), "has space", "semi;colon"} {
		err := Options{Redis: RedisOptions{Prefix: p}}.Validate()
		if err == nil {
			t.Fatalf("prefix %q expected error, got nil", p)
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("prefix %q err = %v, want ErrInvalidOptions", p, err)
		}
	}
}

func TestOptions_ttl_set_applied(t *testing.T) {
	t.Parallel()
	if got := (Options{TTL: 5 * time.Second}).ttl(); got != 5*time.Second {
		t.Fatalf("ttl() = %v, want %v", got, 5*time.Second)
	}
}
