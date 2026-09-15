package media

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
		{"not found", ErrNotFound},
		{"invalid id", ErrInvalidID},
		{"too large", ErrTooLarge},
		{"unsupported format", ErrUnsupportedFormat},
		{"invalid transform", ErrInvalidTransform},
		{"invalid range", ErrInvalidRange},
		{"duration exceeded", ErrDurationExceeded},
		{"tool missing", ErrToolMissing},
		{"probe failed", ErrProbeFailed},
		{"transcode failed", ErrTranscodeFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(tc.err.Error(), "media: ") {
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

	err := &UnknownAdapterError{Adapter: S3}
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

func TestNotFoundError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &NotFoundError{ID: "asset_123"}
	if got := err.Error(); !strings.Contains(got, ErrNotFound.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrNotFound.Error())
	}

	if !strings.Contains(err.Error(), "asset_123") {
		t.Errorf("Error() = %q, want contain asset id", err.Error())
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(%v, ErrNotFound) = false, want true", err)
	}

	var target *NotFoundError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidIDError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidIDError{ID: "../evil"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidID.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidID.Error())
	}

	if !strings.Contains(err.Error(), "../evil") {
		t.Errorf("Error() = %q, want contain asset id", err.Error())
	}

	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("errors.Is(%v, ErrInvalidID) = false, want true", err)
	}

	var target *InvalidIDError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestSizeLimitError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &SizeLimitError{Size: 100, Limit: 10}
	if got := err.Error(); !strings.Contains(got, ErrTooLarge.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrTooLarge.Error())
	}

	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("errors.Is(%v, ErrTooLarge) = false, want true", err)
	}

	var target *SizeLimitError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}

	if target.Size != 100 || target.Limit != 10 {
		t.Errorf("carried values = {%d %d}, want {100 10}", target.Size, target.Limit)
	}
}

func TestUnsupportedFormatError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &UnsupportedFormatError{Format: "image/tiff"}
	if got := err.Error(); !strings.Contains(got, ErrUnsupportedFormat.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrUnsupportedFormat.Error())
	}

	if !strings.Contains(err.Error(), "image/tiff") {
		t.Errorf("Error() = %q, want contain format", err.Error())
	}

	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("errors.Is(%v, ErrUnsupportedFormat) = false, want true", err)
	}

	var target *UnsupportedFormatError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidTransformError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidTransformError{Reason: "bad quality"}
	if got := err.Error(); !strings.Contains(got, ErrInvalidTransform.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidTransform.Error())
	}

	if !strings.Contains(err.Error(), "bad quality") {
		t.Errorf("Error() = %q, want contain reason", err.Error())
	}

	if !errors.Is(err, ErrInvalidTransform) {
		t.Errorf("errors.Is(%v, ErrInvalidTransform) = false, want true", err)
	}

	var target *InvalidTransformError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}
}

func TestInvalidRangeError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &InvalidRangeError{Offset: 90, Length: 20, Size: 100}
	if got := err.Error(); !strings.Contains(got, ErrInvalidRange.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrInvalidRange.Error())
	}

	if !errors.Is(err, ErrInvalidRange) {
		t.Errorf("errors.Is(%v, ErrInvalidRange) = false, want true", err)
	}

	var target *InvalidRangeError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	}

	if target.Offset != 90 || target.Length != 20 || target.Size != 100 {
		t.Errorf("carried values = {%d %d %d}, want {90 20 100}", target.Offset, target.Length, target.Size)
	}
}
