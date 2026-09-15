package document

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fmtWrap(err error) error {
	return fmt.Errorf("wrap: %w", err)
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
		{"unsupported format", ErrUnsupportedFormat},
		{"source too large", ErrSourceTooLarge},
		{"missing endpoint", ErrMissingEndpoint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(tc.err.Error(), "document: ") {
				t.Errorf("sentinel %q missing prefix", tc.err.Error())
			}
		})
	}
}

func TestDuplicateAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &DuplicateAdapterError{Adapter: Local}
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

func TestUnknownAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &UnknownAdapterError{Adapter: Remote}
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

func TestUnsupportedFormatError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &UnsupportedFormatError{Format: OutputFormat("gif")}
	if got := err.Error(); !strings.Contains(got, ErrUnsupportedFormat.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrUnsupportedFormat.Error())
	}

	if !strings.Contains(err.Error(), "gif") {
		t.Errorf("Error() = %q, want contain format", err.Error())
	}

	if !strings.Contains(err.Error(), "pdf, png, or jpg") {
		t.Errorf("Error() = %q, want contain hint", err.Error())
	}

	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("errors.Is(%v, ErrUnsupportedFormat) = false, want true", err)
	}

	var target *UnsupportedFormatError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestSizeLimitError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &SizeLimitError{Size: 100, Limit: 10}
	if got := err.Error(); !strings.Contains(got, ErrSourceTooLarge.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrSourceTooLarge.Error())
	}

	if !errors.Is(err, ErrSourceTooLarge) {
		t.Errorf("errors.Is(%v, ErrSourceTooLarge) = false, want true", err)
	}

	var target *SizeLimitError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}

	if target.Size != 100 || target.Limit != 10 {
		t.Errorf("carried values = {%d %d}, want {100 10}", target.Size, target.Limit)
	}
}
