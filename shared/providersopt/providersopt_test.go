package providersopt

import (
	"strings"
	"testing"
)

func TestValidateEndpoint_empty(t *testing.T) {
	t.Parallel()

	if errs := ValidateEndpoint(""); errs != nil {
		t.Fatalf("ValidateEndpoint(\"\") = %v, want nil", errs)
	}
}

func TestValidateEndpoint_valid(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"https://example.com/hook", "http://localhost:8080"} {
		if errs := ValidateEndpoint(endpoint); errs != nil {
			t.Errorf("ValidateEndpoint(%q) = %v, want nil", endpoint, errs)
		}
	}
}

func TestValidateEndpoint_violations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		want     []string
	}{
		{"no scheme", "example.com/hook", []string{"must include scheme", "must include host"}},
		{"no host", "https:///path", []string{"must include host"}},
		{"garbage", "http://[::1", []string{"valid url"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errs := ValidateEndpoint(tc.endpoint)
			if len(errs) != len(tc.want) {
				t.Fatalf("ValidateEndpoint(%q) = %v, want %d error(s) matching %v", tc.endpoint, errs, len(tc.want), tc.want)
			}

			for i, want := range tc.want {
				if !strings.Contains(errs[i].Error(), want) {
					t.Errorf("errs[%d] = %q, want it to contain %q", i, errs[i].Error(), want)
				}
			}
		})
	}
}

func TestPaddleEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		sandbox  bool
		want     string
	}{
		{"explicit endpoint wins", "https://custom.example.com", true, "https://custom.example.com"},
		{"sandbox default", "", true, "https://sandbox-api.paddle.com"},
		{"production default", "", false, "https://api.paddle.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := PaddleEndpoint(tc.endpoint, tc.sandbox); got != tc.want {
				t.Errorf("PaddleEndpoint(%q, %v) = %q, want %q", tc.endpoint, tc.sandbox, got, tc.want)
			}
		})
	}
}
