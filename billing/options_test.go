package billing

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_violations_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    Options
		reasons []string
	}{
		{"no scheme", Options{Endpoint: "example.com/hook"}, []string{"valid URL"}},
		{"no host", Options{Endpoint: "https:///path"}, []string{"valid URL"}},
		{"garbage", Options{Endpoint: "http://[::1"}, []string{"valid URL"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
			}

			for _, r := range tc.reasons {
				if !strings.Contains(err.Error(), r) {
					t.Errorf("Validate err %q missing %q", err.Error(), r)
				}
			}

			var ioe *InvalidOptionsError
			if !errors.As(err, &ioe) {
				t.Errorf("err %T is not *InvalidOptionsError", err)
			}
		})
	}
}

func TestOptions_Validate_multiple_joined(t *testing.T) {
	t.Parallel()

	err := Options{Endpoint: "example.com/hook"}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}

	if !strings.Contains(err.Error(), "valid URL") {
		t.Errorf("err %q missing endpoint reason", err.Error())
	}
}

func TestOptions_Validate_endpoints_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"https", "https://example.com/hook", false},
		{"http localhost", "http://localhost:8080", false},
		{"empty", "", false},
		{"no scheme", "example.com/hook", true},
		{"no host", "https:///path", true},
		{"garbage", "http://[::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Options{Endpoint: tc.endpoint}.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want error", tc.endpoint)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.endpoint, err)
			}
		})
	}
}

func TestOptions_defaults_values(t *testing.T) {
	t.Parallel()

	if DefaultHTTPTimeout != 30*time.Second {
		t.Errorf("DefaultHTTPTimeout = %v, want %v", DefaultHTTPTimeout, 30*time.Second)
	}
}
