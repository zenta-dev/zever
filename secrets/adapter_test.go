package secrets

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
		{name: "env", in: Env, want: "env"},
		{name: "vault", in: Vault, want: "vault"},
		{name: "gcp", in: GCP, want: "gcp"},
		{name: "aws", in: AWS, want: "aws"},
		{name: "unknown formats", in: Adapter(99), want: "Adapter(99)"},
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
		{in: "env", want: Env},
		{in: "vault", want: Vault},
		{in: "gcp", want: GCP},
		{in: "aws", want: AWS},
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
		{name: "bogus", in: "bogus"},
		{name: "empty", in: ""},
		{name: "case sensitive", in: "Env"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tt.in)
			if err == nil {
				t.Fatal("ParseAdapter() = nil, want ErrInvalidAdapter")
			}

			if got != Env {
				t.Errorf("ParseAdapter() = %v, want Env fallback", got)
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
