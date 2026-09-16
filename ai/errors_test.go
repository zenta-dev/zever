package ai

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSentinels_messages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil factory", ErrNilFactory, "ai: nil factory"},
		{"duplicate", ErrDuplicateAdapter, "ai: duplicate adapter"},
		{"unknown", ErrUnknownAdapter, "ai: unknown adapter"},
		{"invalid adapter", ErrInvalidAdapter, "ai: invalid adapter"},
		{"invalid options", ErrInvalidOptions, "ai: invalid options"},
		{"model not found", ErrModelNotFound, "ai: model not found"},
		{"not supported", ErrNotSupported, "ai: not supported"},
		{"invalid request", ErrInvalidRequest, "ai: invalid request"},
		{"auth", ErrAuth, "ai: auth failed"},
		{"rate limited", ErrRateLimited, "ai: rate limited"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInvalidAdapterError(t *testing.T) {
	t.Parallel()
	err := &InvalidAdapterError{Adapter: "bogus"}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("Is = false, want true")
	}
	var target *InvalidAdapterError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if target.Adapter != "bogus" {
		t.Errorf("Adapter = %q, want %q", target.Adapter, "bogus")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Error() %q missing bogus", err.Error())
	}
	if !strings.Contains(err.Error(), ErrInvalidAdapter.Error()) {
		t.Errorf("Error() %q missing sentinel", err.Error())
	}
}

func TestDuplicateAdapterError(t *testing.T) {
	t.Parallel()
	err := &DuplicateAdapterError{Adapter: OpenAI}
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("Is = false")
	}
	var target *DuplicateAdapterError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if target.Adapter != OpenAI {
		t.Errorf("Adapter = %v, want %v", target.Adapter, OpenAI)
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Errorf("Error() %q missing openai", err.Error())
	}
}

func TestUnknownAdapterError(t *testing.T) {
	t.Parallel()
	err := &UnknownAdapterError{Adapter: Adapter(123)}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Is = false")
	}
	var target *UnknownAdapterError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if target.Adapter != Adapter(123) {
		t.Errorf("Adapter = %v, want 123", target.Adapter)
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("Error() %q missing unknown", err.Error())
	}
}

func TestInvalidOptionsError(t *testing.T) {
	t.Parallel()
	err := &InvalidOptionsError{Reason: "timeout must be >= 0"}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Is = false")
	}
	var target *InvalidOptionsError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if target.Reason != "timeout must be >= 0" {
		t.Errorf("Reason = %q", target.Reason)
	}
	if !strings.Contains(err.Error(), "timeout must be >= 0") {
		t.Errorf("Error() %q missing reason", err.Error())
	}
}

func TestRateLimitedError_zero(t *testing.T) {
	t.Parallel()
	err := &RateLimitedError{}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Is = false")
	}
	var target *RateLimitedError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if err.Error() != ErrRateLimited.Error() {
		t.Errorf("Error() = %q, want %q", err.Error(), ErrRateLimited.Error())
	}
	if target.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0", target.RetryAfter)
	}
}

func TestRateLimitedError_withRetry(t *testing.T) {
	t.Parallel()
	d := 2 * time.Second
	err := &RateLimitedError{RetryAfter: d}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Is = false")
	}
	msg := err.Error()
	if !strings.Contains(msg, "retry after") {
		t.Errorf("Error() %q missing retry after", msg)
	}
	if !strings.Contains(msg, "2s") {
		t.Errorf("Error() %q missing 2s", msg)
	}
	var target *RateLimitedError
	if !errors.As(err, &target) {
		t.Fatalf("As failed")
	}
	if target.RetryAfter != d {
		t.Errorf("RetryAfter = %v, want %v", target.RetryAfter, d)
	}
}

func TestErrors_Unwrap_chain(t *testing.T) {
	t.Parallel()
	// Ensure errors.Is works through fmt.Errorf wrapping.
	base := &InvalidOptionsError{Reason: "x"}
	wrapped := errors.Join(base)
	if !errors.Is(wrapped, ErrInvalidOptions) {
		t.Fatalf("Join Is = false")
	}
}
