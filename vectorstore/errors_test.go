package vectorstore

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
		{"empty embedding", ErrEmptyEmbedding},
		{"dimension mismatch", ErrDimensionMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !strings.HasPrefix(tc.err.Error(), "vectorstore: ") {
				t.Errorf("sentinel %q missing prefix", tc.err.Error())
			}

			if tc.err.Error() != strings.ToLower(tc.err.Error()) {
				t.Errorf("sentinel %q not lowercase", tc.err.Error())
			}
		})
	}
}

func TestSentinel_exactMessages(t *testing.T) {
	t.Parallel()

	if ErrNotFound.Error() != "vectorstore: not found" {
		t.Errorf("ErrNotFound = %q, want %q", ErrNotFound.Error(), "vectorstore: not found")
	}
}

func TestDuplicateAdapterError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &DuplicateAdapterError{Adapter: SQLite}
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

	err := &UnknownAdapterError{Adapter: PGVector}
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

	err := &NotFoundError{ID: "vec-1"}
	if got := err.Error(); !strings.Contains(got, ErrNotFound.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrNotFound.Error())
	}

	if !strings.Contains(err.Error(), `"vec-1"`) {
		t.Errorf("Error() = %q, want contain quoted id", err.Error())
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(%v, ErrNotFound) = false, want true", err)
	}

	var target *NotFoundError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	} else if target.ID != "vec-1" {
		t.Errorf("carried ID = %q, want %q", target.ID, "vec-1")
	}
}

func TestDimensionMismatchError_message_unwrap(t *testing.T) {
	t.Parallel()

	err := &DimensionMismatchError{Got: 3, Want: 1536}
	got := err.Error()
	if !strings.Contains(got, ErrDimensionMismatch.Error()) {
		t.Errorf("Error() = %q, want contain %q", got, ErrDimensionMismatch.Error())
	}

	if !strings.Contains(got, "got 3, want 1536") {
		t.Errorf("Error() = %q, want contain dimensions", got)
	}

	if !errors.Is(err, ErrDimensionMismatch) {
		t.Errorf("errors.Is(%v, ErrDimensionMismatch) = false, want true", err)
	}

	var target *DimensionMismatchError
	if !errors.As(fmtWrap(err), &target) {
		t.Errorf("errors.As failed for %T", err)
	} else if target.Got != 3 || target.Want != 1536 {
		t.Errorf("carried = %+v, want {3 1536}", target)
	}
}
