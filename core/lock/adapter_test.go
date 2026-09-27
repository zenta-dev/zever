package lock

import (
	"errors"
	"testing"
)

func TestAdapterString_returnsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Adapter
		want string
	}{
		{name: "memory", in: Memory, want: "memory"},
		{name: "redis", in: Redis, want: "redis"},
		{name: "unknown formats", in: Adapter(""), want: "unknown"},
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
		in   string
		want Adapter
	}{
		{in: "memory", want: Memory},
		{in: "redis", want: Redis},
	}

	for _, tt := range tests {
		got, err := ParseAdapter(tt.in)
		if err != nil {
			t.Fatalf("ParseAdapter(%q) error = %v", tt.in, err)
		}

		if got != tt.want {
			t.Errorf("ParseAdapter(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseAdapter_invalid_returnsInvalidAdapterError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tt.in)
			if err == nil {
				t.Fatal("ParseAdapter() = nil, want ErrInvalidAdapter")
			}

			if got != Adapter("") {
				t.Errorf("ParseAdapter() = %v, want empty fallback", got)
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
func TestParseAdapter_open_acceptsCustom(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"bogus", "Memory", "memory "} {
		got, err := ParseAdapter(in)
		if err != nil {
			t.Fatalf("ParseAdapter(%q) error = %v, want nil (open adapter)", in, err)
		}
		if got != Adapter(in) {
			t.Errorf("ParseAdapter(%q) = %v, want %v", in, got, Adapter(in))
		}
	}
}
