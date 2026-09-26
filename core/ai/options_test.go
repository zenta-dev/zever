package ai

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_Validate_zero(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts Options
	}{
		{"https base", Options{BaseURL: "https://api.openai.com/v1", Timeout: 0}},
		{"https with path", Options{BaseURL: "https://example.com/api", Timeout: 5 * time.Second}},
		{"empty base", Options{BaseURL: ""}},
		{"zero timeout", Options{Timeout: 0}},
		{"positive timeout", Options{Timeout: time.Second}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.opts.Validate(); err != nil {
				t.Errorf("Validate(%v) = %v, want nil", tc.opts, err)
			}
		})
	}
}

func TestOptions_Validate_timeout(t *testing.T) {
	t.Parallel()
	err := Options{Timeout: -1}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err %q missing timeout", err.Error())
	}
	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_Validate_baseURL_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		baseURL string
		wantErr bool
		reasons []string
	}{
		{"valid https", "https://api.openai.com/v1", false, nil},
		{"valid https port", "https://example.com:443/v1", false, nil},
		{"empty", "", false, nil},
		{"no scheme", "example.com/api", true, []string{"scheme"}},
		{"no host", "https:///path", true, []string{"host"}},
		{"garbage", "http://[::1", true, []string{"valid url"}},
		{"http blocked", "http://example.com/v1", true, []string{"https"}},
		{"http localhost blocked", "http://localhost:8080", true, []string{"https"}},
		{"ftp blocked", "ftp://example.com/v1", true, []string{"https"}},
		{"scheme only host missing http", "http://", true, []string{"host"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Options{BaseURL: tc.baseURL}.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.baseURL)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", tc.baseURL, err)
			}
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidOptions) {
					t.Fatalf("err = %v, want ErrInvalidOptions", err)
				}
				for _, r := range tc.reasons {
					if !strings.Contains(err.Error(), r) {
						t.Errorf("err %q missing %q", err.Error(), r)
					}
				}
			}
		})
	}
}

func TestOptions_Validate_multiple_joined(t *testing.T) {
	t.Parallel()
	err := Options{Timeout: -5, BaseURL: "http://example.com"}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err %q missing timeout", err.Error())
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("err %q missing https", err.Error())
	}
	// Ensure errors.Join produces join with multiple underlying errors.
	// Validate via errors.Is still true for ErrInvalidOptions multiple times.
}

func TestOptions_Validate_boundary(t *testing.T) {
	t.Parallel()
	// Timeout 0 is boundary valid.
	if err := (Options{Timeout: 0}).Validate(); err != nil {
		t.Errorf("Timeout 0 err = %v, want nil", err)
	}
	// Timeout -1nanos boundary invalid.
	if err := (Options{Timeout: -time.Nanosecond}).Validate(); err == nil {
		t.Errorf("Timeout -1ns = nil, want error")
	}
}
