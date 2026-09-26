package geo

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
		{name: "google", adapter: Google, want: "google"},
		{name: "static", adapter: Static, want: "static"},
		{name: "osm", adapter: OSM, want: "osm"},
		{name: "unknown", adapter: Adapter(""), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.adapter.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
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
		{name: "google", input: "google", want: Google},
		{name: "static", input: "static", want: Static},
		{name: "osm", input: "osm", want: OSM},
		{name: "unknown", input: "bogus", want: Adapter("bogus")},
		{name: "case sensitive", input: "Google", want: Adapter("Google")},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAdapter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidAdapter) {
					t.Fatalf("expected errors.Is ErrInvalidAdapter, got %v", err)
				}
				var invErr *InvalidAdapterError
				if !errors.As(err, &invErr) {
					t.Fatalf("expected errors.As InvalidAdapterError, got %T", err)
				}
				if invErr.Adapter != tt.input {
					t.Fatalf("expected adapter %q, got %q", tt.input, invErr.Adapter)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseAdapter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
