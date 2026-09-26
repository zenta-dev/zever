package db

import (
	"errors"
	"testing"
)

func TestAdapterString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		adapter Adapter
		want    string
	}{
		{name: "sqlite", adapter: SQLite, want: "sqlite"},
		{name: "postgres", adapter: Postgres, want: "postgres"},
		{name: "unknown negative", adapter: Adapter(-1), want: "unknown"},
		{name: "unknown large", adapter: Adapter(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.adapter.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Adapter
		wantErr bool
	}{
		{name: "sqlite", input: "sqlite", want: SQLite},
		{name: "postgres", input: "postgres", want: Postgres},
		{name: "invalid", input: "mysql", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "uppercase rejected", input: "SQLite", wantErr: true},
		{name: "whitespace rejected", input: " sqlite", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				var invErr *InvalidAdapterError
				if !errors.As(err, &invErr) {
					t.Fatalf("error is %T, want *InvalidAdapterError", err)
				}

				if !errors.Is(err, ErrInvalidAdapter) {
					t.Errorf("error does not unwrap to ErrInvalidAdapter: %v", err)
				}

				if invErr.Adapter != tt.input {
					t.Errorf("Adapter = %q, want %q", invErr.Adapter, tt.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("ParseAdapter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
