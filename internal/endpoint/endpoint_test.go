package endpoint

import (
	"errors"
	"testing"
)

func TestValidateURL_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		opts    []Option
		wantErr error // nil means success; else errors.Is match
	}{
		{"https ok", "https://example.com/v1", nil, nil},
		{"https with port", "https://example.com:443/v1", nil, nil},
		{"empty", "", nil, ErrEmpty},
		{"parse error", "http://[::1", nil, ErrParse},
		{"no scheme", "example.com/api", nil, ErrNoScheme},
		{"no host", "https:///path", nil, ErrNoHost},
		{"scheme only", "http://", nil, ErrNoHost},
		{"http blocked by default", "http://example.com", nil, ErrInsecureScheme},
		{"http localhost blocked by default", "http://localhost:8080", nil, ErrInsecureScheme},
		{"ftp blocked", "ftp://example.com", nil, ErrUnsupportedScheme},
		{"http allowed insecure", "http://example.com", []Option{WithAllowInsecure(true)}, nil},
		{"http denied insecure false", "http://example.com", []Option{WithAllowInsecure(false)}, ErrInsecureScheme},
		{"https with insecure", "https://example.com", []Option{WithAllowInsecure(true)}, nil},
		{"ftp denied even insecure", "ftp://example.com", []Option{WithAllowInsecure(true)}, ErrUnsupportedScheme},
		{"loopback http v4", "http://127.0.0.1:8080", []Option{WithAllowLoopbackHTTP()}, nil},
		{"loopback http localhost", "http://localhost:8080", []Option{WithAllowLoopbackHTTP()}, nil},
		{"loopback http v6", "http://[::1]:8080", []Option{WithAllowLoopbackHTTP()}, nil},
		{"loopback http 127 full /8", "http://127.0.0.2:8080", []Option{WithAllowLoopbackHTTP()}, nil},
		{"non-loopback http denied", "http://example.com", []Option{WithAllowLoopbackHTTP()}, ErrInsecureScheme},
		{"spoofed loopback suffix denied", "http://127.0.0.1.evil.com", []Option{WithAllowLoopbackHTTP()}, ErrInsecureScheme},
		{"spoofed localhost suffix denied", "http://localhost.evil.com", []Option{WithAllowLoopbackHTTP()}, ErrInsecureScheme},
		{"https unaffected by loopback opt", "https://example.com", []Option{WithAllowLoopbackHTTP()}, nil},
		{"userinfo allowed by default", "https://user@example.com", nil, nil},
		{"userinfo rejected", "http://user@example.com", []Option{WithAllowInsecure(true), WithRejectUserinfo()}, ErrUserinfo},
		{"whitespace rejected", "http://local host:11434", []Option{WithAllowInsecure(true), WithRejectWhitespace()}, ErrWhitespace},
		{"tab rejected", "http://example.com/\tx", []Option{WithRejectWhitespace()}, ErrWhitespace},
		{"whitespace allowed by default", "https://example.com/a b", nil, nil},
		{"query allowed by default", "https://example.com?token=1", nil, nil},
		{"fragment allowed by default", "https://example.com/#frag", nil, nil},
		{"query rejected", "https://example.com?token=1", []Option{WithRejectQueryFragment()}, ErrQueryFragment},
		{"fragment rejected", "https://example.com/#frag", []Option{WithRejectQueryFragment()}, ErrQueryFragment},
		{"query+fragment rejected", "https://example.com/p?x=1#f", []Option{WithRejectQueryFragment()}, ErrQueryFragment},
		{"any scheme ftp", "ftp://example.com/x", []Option{WithAllowAnyScheme()}, nil},
		{"any scheme still needs scheme", "example.com/x", []Option{WithAllowAnyScheme()}, ErrNoScheme},
		{"any scheme still needs host", "ftp:///x", []Option{WithAllowAnyScheme()}, ErrNoHost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateURL(tc.raw, tc.opts...)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateURL(%q) = %v, want nil", tc.raw, err)
				}
				if got == "" {
					t.Fatalf("ValidateURL(%q) returned empty normalized URL", tc.raw)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateURL(%q) = %q, want error %v", tc.raw, got, tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateURL(%q) = %v, want %v", tc.raw, err, tc.wantErr)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateURL(%q) = %v, want wrap of ErrInvalid", tc.raw, err)
			}
		})
	}
}

func TestValidateURL_Normalizes(t *testing.T) {
	t.Parallel()
	got, err := ValidateURL("https://example.com/render")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/render" {
		t.Fatalf("got %q", got)
	}
}

func TestIsLoopbackHost_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		{"example.com", false},
		{"127.0.0.1.evil.com", false},
		{"localhost.evil.com", false},
		{"evil-localhost.com", false},
		{"", false},
		{"8.8.8.8", false},
	}
	for _, tc := range cases {
		if got := IsLoopbackHost(tc.host); got != tc.want {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestIsLoopbackURL(t *testing.T) {
	t.Parallel()
	if !IsLoopbackURL("http://127.0.0.1:8080/x") {
		t.Error("want true for 127.0.0.1")
	}
	if !IsLoopbackURL("https://localhost/x") {
		t.Error("want true for localhost")
	}
	if IsLoopbackURL("https://127.0.0.1.evil.com") {
		t.Error("want false for spoofed suffix")
	}
	if IsLoopbackURL("http://[::1") {
		t.Error("want false for unparseable")
	}
	if IsLoopbackURL("") {
		t.Error("want false for empty")
	}
}
