package log

import (
	"errors"
	"testing"
)

func TestAdapter_String_returnsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Adapter
		want string
	}{
		{name: "noop", in: Noop, want: "noop"},
		{name: "zerolog", in: ZeroLog, want: "zerolog"},
		{name: "slog", in: Slog, want: "slog"},
		{name: "unknown", in: Adapter(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.in.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAdapter_valid_returnsAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want Adapter
	}{
		{name: "noop", in: "noop", want: Noop},
		{name: "zerolog", in: "zerolog", want: ZeroLog},
		{name: "slog", in: "slog", want: Slog},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tt.in)
			if err != nil {
				t.Fatalf("ParseAdapter() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("ParseAdapter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAdapter_invalid_returnsInvalidAdapterError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "bogus", in: "bogus"},
		{name: "uppercase rejected", in: "NOOP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tt.in)
			if err == nil {
				t.Fatal("ParseAdapter() error = nil, want ErrInvalidAdapter")
			}

			if got != Noop {
				t.Errorf("ParseAdapter() adapter = %v, want Noop zero fallback", got)
			}

			if !errors.Is(err, ErrInvalidAdapter) {
				t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
			}

			var invErr *InvalidAdapterError
			if !errors.As(err, &invErr) {
				t.Fatalf("errors.As(err, InvalidAdapterError) = false (err = %T %v)", err, err)
			}

			if invErr.Adapter != tt.in {
				t.Errorf("InvalidAdapterError.Adapter = %q, want %q", invErr.Adapter, tt.in)
			}
		})
	}
}
