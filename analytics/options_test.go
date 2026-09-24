package analytics

import (
	"errors"
	"strings"
	"testing"
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
		{"negative bytes", Options{MaxPropertiesBytes: -1}, []string{"max_properties_bytes"}},
		{"negative count", Options{MaxProperties: -1}, []string{"max_properties must"}},
		{"no scheme", Options{Endpoint: "example.com/track"}, []string{"scheme"}},
		{"no host", Options{Endpoint: "https:///path"}, []string{"host"}},
		{"garbage", Options{Endpoint: "http://[::1"}, []string{"valid url"}},
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
	err := Options{MaxPropertiesBytes: -1, MaxProperties: -2}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "max_properties_bytes") {
		t.Errorf("err %q missing bytes reason", err.Error())
	}
	if !strings.Contains(err.Error(), "max_properties must") {
		t.Errorf("err %q missing count reason", err.Error())
	}
}

func TestOptions_Validate_endpoints_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"https", "https://example.com/track", false},
		{"http localhost", "http://localhost:8080", false},
		{"empty", "", false},
		{"no scheme", "example.com/track", true},
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

func TestOptions_accessors_defaults_table(t *testing.T) {
	t.Parallel()
	if got := (Options{}).anonymousID(); got != DefaultAnonymousID {
		t.Errorf("anonymousID() = %q, want %q", got, DefaultAnonymousID)
	}
	if got := (Options{AnonymousID: "u1"}).anonymousID(); got != "u1" {
		t.Errorf("anonymousID() = %q, want %q", got, "u1")
	}
	if got := (Options{}).groupType(); got != DefaultGroupType {
		t.Errorf("groupType() = %q, want %q", got, DefaultGroupType)
	}
	if got := (Options{GroupType: "team"}).groupType(); got != "team" {
		t.Errorf("groupType() = %q, want %q", got, "team")
	}
	if got := (Options{}).maxPropertiesBytes(); got != DefaultMaxPropertiesBytes {
		t.Errorf("maxPropertiesBytes() = %d, want %d", got, DefaultMaxPropertiesBytes)
	}
	if got := (Options{MaxPropertiesBytes: -5}).maxPropertiesBytes(); got != DefaultMaxPropertiesBytes {
		t.Errorf("maxPropertiesBytes() = %d, want default", got)
	}
	if got := (Options{MaxPropertiesBytes: 123}).maxPropertiesBytes(); got != 123 {
		t.Errorf("maxPropertiesBytes() = %d, want 123", got)
	}
}
