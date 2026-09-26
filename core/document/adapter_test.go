package document

import (
	"errors"
	"testing"
)

func TestAdapter_String_values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		adapter Adapter
		want    string
	}{
		{"local", Local, "local"},
		{"remote", Remote, "remote"},
		{"latex", Latex, "latex"},
		{"unknown", Adapter(""), "unknown"},
		{"negative", Adapter(""), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.adapter.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAdapter_Parse_valid_roundtrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want Adapter
	}{
		{"local", Local},
		{"remote", Remote},
		{"latex", Latex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(tc.name)
			if err != nil {
				t.Fatalf("ParseAdapter(%q) err = %v", tc.name, err)
			}

			if got != tc.want {
				t.Errorf("ParseAdapter(%q) = %v, want %v", tc.name, got, tc.want)
			}

			if got.String() != tc.name {
				t.Errorf("roundtrip String() = %q, want %q", got.String(), tc.name)
			}
		})
	}
}

func TestAdapter_Parse_invalid_returnsLocalAndInvalidAdapter(t *testing.T) {
	t.Parallel()

	for _, in := range []string{""} {
		t.Run("input:"+in, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAdapter(in)
			if !errors.Is(err, ErrInvalidAdapter) {
				t.Fatalf("ParseAdapter(%q) err = %v, want ErrInvalidAdapter", in, err)
			}

			var iae *InvalidAdapterError
			if !errors.As(err, &iae) {
				t.Fatalf("ParseAdapter(%q) err %T is not *InvalidAdapterError", in, err)
			}

			if iae.Adapter != in {
				t.Errorf("carried Adapter = %q, want %q", iae.Adapter, in)
			}

			if got != Adapter("") {
				t.Errorf("ParseAdapter(%q) = %v, want empty on failure", in, got)
			}
		})
	}
}
