package redis

import (
	"strings"
	"testing"
)

func TestValidateAddr(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"", "localhost:6379", "10.0.0.1:6380", "redis:6379"} {
		if err := ValidateAddr(addr); err != nil {
			t.Errorf("ValidateAddr(%q) = %v, want nil", addr, err)
		}
	}

	for _, addr := range []string{
		"https://h:6379",
		"redis://localhost:6379",
		"h:6379/x",
		"h:6379?q=1",
		"h:6379#frag",
		"h",
		":0",
		"h:99999",
		"h:0",
		"h:-1",
		"h:notaport",
		":6379",
	} {
		if err := ValidateAddr(addr); err == nil {
			t.Errorf("ValidateAddr(%q) = nil, want error", addr)
		}
	}
}

func TestValidateAddr_reasons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		addr string
		want string
	}{
		{"redis://h:6379", "redis: addr must be host:port without scheme"},
		{"h:6379/x", "redis: addr must be host:port"},
		{":6379", "redis: addr host must be non-empty"},
		{"h:99999", "redis: addr port must be 1-65535"},
	}

	for _, tc := range cases {
		err := ValidateAddr(tc.addr)
		if err == nil {
			t.Fatalf("ValidateAddr(%q) = nil, want error", tc.addr)
		}

		if err.Error() != tc.want {
			t.Errorf("ValidateAddr(%q) = %q, want %q", tc.addr, err.Error(), tc.want)
		}
	}
}

func TestValidatePrefix(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"", "sess", "a!b_c.d", "lock"} {
		if err := ValidatePrefix(p); err != nil {
			t.Errorf("ValidatePrefix(%q) = %v, want nil", p, err)
		}
	}

	if err := ValidatePrefix(strings.Repeat("p", 65)); err == nil {
		t.Error("ValidatePrefix(65 chars) = nil, want error")
	} else if err.Error() != "redis: prefix must be at most 64 characters" {
		t.Errorf("ValidatePrefix(65 chars) = %q, want length reason", err.Error())
	}

	for _, p := range []string{"has space", "semi;colon", "colon:prefix", "slash/x"} {
		if err := ValidatePrefix(p); err == nil {
			t.Errorf("ValidatePrefix(%q) = nil, want error", p)
		} else if err.Error() != "redis: prefix must contain only token characters" {
			t.Errorf("ValidatePrefix(%q) = %q, want token reason", p, err.Error())
		}
	}

	if err := ValidatePrefix(strings.Repeat("p", 64)); err != nil {
		t.Errorf("ValidatePrefix(64 chars) = %v, want nil", err)
	}
}

func TestIsTokenChar(t *testing.T) {
	t.Parallel()

	for _, c := range []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		if !IsTokenChar(c) {
			t.Errorf("IsTokenChar(%q) = false, want true", c)
		}
	}

	for _, c := range []byte{'!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~'} {
		if !IsTokenChar(c) {
			t.Errorf("IsTokenChar(%q) = false, want true", c)
		}
	}

	for _, c := range []byte{'(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}', ' ', '\t', 0x00, 0x1f, 0x7f, 0x80} {
		if IsTokenChar(c) {
			t.Errorf("IsTokenChar(%q) = true, want false", c)
		}
	}
}

func TestRedactAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		addr string
		want string
	}{
		{"", ""},
		{"localhost:6379", "localhost:6379"},
		{"  localhost:6379  ", "localhost:6379"},
		{"redis://:s3cret@h:6379", "redis://:xxxxx@h:6379"},
		{"redis://bob:s3cret@h:6379", "redis://bob:xxxxx@h:6379"},
		{"redis://[::1", "redis://[::1"},
	}

	for _, tc := range cases {
		if got := RedactAddr(tc.addr); got != tc.want {
			t.Errorf("RedactAddr(%q) = %q, want %q", tc.addr, got, tc.want)
		}

		if strings.Contains(RedactAddr(tc.addr), "s3cret") {
			t.Errorf("RedactAddr(%q) leaks password", tc.addr)
		}
	}
}

func TestRedactEndpoint_urlPrecedence(t *testing.T) {
	t.Parallel()

	if got := RedactEndpoint("redis://h1:6379", "h2:6379"); got != "redis://h1:6379" {
		t.Errorf("RedactEndpoint(url, addr) = %q, want url", got)
	}

	if got := RedactEndpoint("", "h2:6379"); got != "h2:6379" {
		t.Errorf("RedactEndpoint(empty, addr) = %q, want addr", got)
	}

	if got := RedactEndpoint("redis://:s3cret@h:6379", "plain:6379"); got != "redis://:xxxxx@h:6379" {
		t.Errorf("RedactEndpoint() = %q, want masked url", got)
	}
}
