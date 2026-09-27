package analytics

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fmtWrap(err error) error {
	return fmt.Errorf("wrap: %w", err)
}

func TestDuplicateAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &DuplicateAdapterError{Adapter: Log}
	if got := err.Error(); !strings.Contains(got, ErrDuplicateAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrDuplicateAdapter.Error())
	}
	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Errorf("errors.Is(%v, ErrDuplicateAdapter) = false, want true", err)
	}
	var target *DuplicateAdapterError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestDuplicateAliases_compat(t *testing.T) {
	t.Parallel()
	err := &DuplicateAdapterError{Adapter: Log}
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("errors.Is(%v, ErrDuplicate) = false, want true", err)
	}
	var target *DuplicateError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T alias", err)
	}
	if !errors.Is(ErrDuplicateAdapter, ErrDuplicate) {
		t.Errorf("alias ErrDuplicate does not match ErrDuplicateAdapter")
	}
}

func TestUnknownAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &UnknownAdapterError{Adapter: PostHog}
	if got := err.Error(); !strings.Contains(got, ErrUnknownAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrUnknownAdapter.Error())
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("errors.Is(%v, ErrUnknownAdapter) = false, want true", err)
	}
	var target *UnknownAdapterError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &InvalidAdapterError{Adapter: "bogus"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidAdapter.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidAdapter.Error())
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Error() = %q, want contain adapter name", err.Error())
	}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Errorf("errors.Is(%v, ErrInvalidAdapter) = false, want true", err)
	}
	var target *InvalidAdapterError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidOptionsError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &InvalidOptionsError{Reason: "boom"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidOptions.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidOptions.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("Error() = %q, want contain reason", err.Error())
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("errors.Is(%v, ErrInvalidOptions) = false, want true", err)
	}
	var target *InvalidOptionsError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestCountLimitError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &CountLimitError{Count: 5, Limit: 3}
	if got := err.Error(); !strings.Contains(got, ErrTooManyProperties.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrTooManyProperties.Error())
	}
	if !errors.Is(err, ErrTooManyProperties) {
		t.Errorf("errors.Is(%v, ErrTooManyProperties) = false, want true", err)
	}
	var target *CountLimitError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestSizeLimitError_message_unwrap(t *testing.T) {
	t.Parallel()
	err := &SizeLimitError{Size: 100, Limit: 10}
	if got := err.Error(); !strings.Contains(got, ErrPropertiesTooLarge.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrPropertiesTooLarge.Error())
	}
	if !errors.Is(err, ErrPropertiesTooLarge) {
		t.Errorf("errors.Is(%v, ErrPropertiesTooLarge) = false, want true", err)
	}
	var target *SizeLimitError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestSentinels_messages_prefixed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{"nil factory", ErrNilFactory},
		{"duplicate", ErrDuplicateAdapter},
		{"unknown", ErrUnknownAdapter},
		{"invalid adapter", ErrInvalidAdapter},
		{"invalid options", ErrInvalidOptions},
		{"missing identity", ErrMissingIdentity},
		{"missing group", ErrMissingGroupID},
		{"too many", ErrTooManyProperties},
		{"too large", ErrPropertiesTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(tc.err.Error(), "analytics: ") {
				t.Errorf("sentinel %q missing prefix", tc.err.Error())
			}
		})
	}
}
