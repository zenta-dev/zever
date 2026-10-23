package ai

import (
	"errors"
	"strings"
	"testing"
)

func TestEdgeOptionsValidate_userinfoBaseURL(t *testing.T) {
	t.Parallel()

	// Userinfo is permitted by endpoint.ValidateURL defaults.
	if err := (Options{BaseURL: "https://user@example.com/v1"}).Validate(); err != nil {
		t.Fatalf("Validate err = %v, want nil", err)
	}
}

func TestEdgeOptionsValidate_queryAndFragmentBaseURL(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://example.com/v1?q=1",
		"https://example.com/v1#frag",
	}
	for _, baseURL := range cases {
		if err := (Options{BaseURL: baseURL}).Validate(); err != nil {
			t.Errorf("Validate(%q) err = %v, want nil", baseURL, err)
		}
	}
}

func TestEdgeOptionsValidate_loopbackInsecureAllowed(t *testing.T) {
	t.Parallel()

	cases := []string{
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	}
	for _, baseURL := range cases {
		if err := (Options{BaseURL: baseURL, AllowInsecure: true}).Validate(); err != nil {
			t.Errorf("Validate(%q, insecure) err = %v, want nil", baseURL, err)
		}
	}
}

func TestEdgeOptionsValidate_httpsNoHost(t *testing.T) {
	t.Parallel()

	err := (Options{BaseURL: "https://"}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "host") {
		t.Fatalf("err %q missing host reason", err.Error())
	}
}

func TestEdgeRateLimitedError_negativeRetryAfter(t *testing.T) {
	t.Parallel()

	err := RateLimitedError{RetryAfter: -1}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if err.Error() != ErrRateLimited.Error() {
		t.Fatalf("Error() = %q, want plain sentinel for negative RetryAfter", err.Error())
	}
}

func TestEdgeInvalidOptionsError_emptyReason(t *testing.T) {
	t.Parallel()

	err := InvalidOptionsError{}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.HasSuffix(err.Error(), ": ") {
		t.Fatalf("Error() = %q, want trailing sentinel with empty reason", err.Error())
	}
}
