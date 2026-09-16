package geo

import (
	"errors"
	"strings"
	"testing"
)

func TestSentinels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		prefix string
	}{
		{name: "nil factory", err: ErrNilFactory, prefix: "geo: nil factory"},
		{name: "duplicate", err: ErrDuplicateAdapter, prefix: "geo: duplicate adapter"},
		{name: "unknown", err: ErrUnknownAdapter, prefix: "geo: unknown adapter"},
		{name: "invalid adapter", err: ErrInvalidAdapter, prefix: "geo: invalid adapter"},
		{name: "invalid options", err: ErrInvalidOptions, prefix: "geo: invalid options"},
		{name: "not found", err: ErrNotFound, prefix: "geo: not found"},
		{name: "invalid coordinate", err: ErrInvalidCoordinate, prefix: "geo: invalid coordinate"},
		{name: "too large", err: ErrTooLarge, prefix: "geo: response too large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err == nil {
				t.Fatal("sentinel is nil")
			}
			if !strings.HasPrefix(tt.err.Error(), tt.prefix) {
				t.Fatalf("Error() = %q, want prefix %q", tt.err.Error(), tt.prefix)
			}
		})
	}
}

func TestTypedErrors(t *testing.T) {
	t.Parallel()
	t.Run("InvalidAdapterError", func(t *testing.T) {
		t.Parallel()
		err := &InvalidAdapterError{Adapter: "bogus"}
		if !strings.Contains(err.Error(), ErrInvalidAdapter.Error()) {
			t.Fatalf("Error() = %q, want prefix %q", err.Error(), ErrInvalidAdapter)
		}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Fatalf("expected errors.Is ErrInvalidAdapter, got %v", err)
		}
		var target *InvalidAdapterError
		if !errors.As(err, &target) {
			t.Fatalf("expected errors.As InvalidAdapterError, got %T", err)
		}
		if !errors.Is(err.Unwrap(), ErrInvalidAdapter) {
			t.Fatalf("Unwrap() = %v, want %v", err.Unwrap(), ErrInvalidAdapter)
		}
	})

	t.Run("DuplicateAdapterError", func(t *testing.T) {
		t.Parallel()
		err := &DuplicateAdapterError{Adapter: Google}
		if !strings.Contains(err.Error(), ErrDuplicateAdapter.Error()) {
			t.Fatalf("Error() = %q, want prefix %q", err.Error(), ErrDuplicateAdapter)
		}
		if !errors.Is(err, ErrDuplicateAdapter) {
			t.Fatalf("expected errors.Is ErrDuplicateAdapter, got %v", err)
		}
		var target *DuplicateAdapterError
		if !errors.As(err, &target) {
			t.Fatalf("expected errors.As DuplicateAdapterError, got %T", err)
		}
		if !errors.Is(err.Unwrap(), ErrDuplicateAdapter) {
			t.Fatalf("Unwrap() = %v, want %v", err.Unwrap(), ErrDuplicateAdapter)
		}
	})

	t.Run("UnknownAdapterError", func(t *testing.T) {
		t.Parallel()
		err := &UnknownAdapterError{Adapter: Adapter(99)}
		if !strings.Contains(err.Error(), ErrUnknownAdapter.Error()) {
			t.Fatalf("Error() = %q, want prefix %q", err.Error(), ErrUnknownAdapter)
		}
		if !errors.Is(err, ErrUnknownAdapter) {
			t.Fatalf("expected errors.Is ErrUnknownAdapter, got %v", err)
		}
		var target *UnknownAdapterError
		if !errors.As(err, &target) {
			t.Fatalf("expected errors.As UnknownAdapterError, got %T", err)
		}
		if !errors.Is(err.Unwrap(), ErrUnknownAdapter) {
			t.Fatalf("Unwrap() = %v, want %v", err.Unwrap(), ErrUnknownAdapter)
		}
	})

	t.Run("InvalidOptionsError", func(t *testing.T) {
		t.Parallel()
		err := &InvalidOptionsError{Reason: "timeout must be >= 0"}
		if !strings.Contains(err.Error(), ErrInvalidOptions.Error()) {
			t.Fatalf("Error() = %q, want prefix %q", err.Error(), ErrInvalidOptions)
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("expected errors.Is ErrInvalidOptions, got %v", err)
		}
		var target *InvalidOptionsError
		if !errors.As(err, &target) {
			t.Fatalf("expected errors.As InvalidOptionsError, got %T", err)
		}
		if !errors.Is(err.Unwrap(), ErrInvalidOptions) {
			t.Fatalf("Unwrap() = %v, want %v", err.Unwrap(), ErrInvalidOptions)
		}
	})

	t.Run("NotFoundError", func(t *testing.T) {
		t.Parallel()
		err := &NotFoundError{Query: "nowhere"}
		if !strings.Contains(err.Error(), ErrNotFound.Error()) {
			t.Fatalf("Error() = %q, want prefix %q", err.Error(), ErrNotFound)
		}
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected errors.Is ErrNotFound, got %v", err)
		}
		var target *NotFoundError
		if !errors.As(err, &target) {
			t.Fatalf("expected errors.As NotFoundError, got %T", err)
		}
		if !errors.Is(err.Unwrap(), ErrNotFound) {
			t.Fatalf("Unwrap() = %v, want %v", err.Unwrap(), ErrNotFound)
		}
	})
}
