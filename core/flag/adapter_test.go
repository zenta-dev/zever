package flag

import (
	"errors"
	"testing"
)

func TestAdapter_String_returnsName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		adapter Adapter
		want    string
	}{
		{Static, "static"},
		{Firebase, "firebase"},
		{Adapter(""), "unknown"},
	}
	for _, c := range cases {
		if got := c.adapter.String(); got != c.want {
			t.Errorf("Adapter(%q).String() = %q want %q", string(c.adapter), got, c.want)
		}
	}
}

func TestAdapter_Parse_valid_roundtrip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want Adapter
	}{
		{"static", Static},
		{"firebase", Firebase},
	} {
		got, err := ParseAdapter(tc.in)
		if err != nil {
			t.Fatalf("ParseAdapter(%q) err = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseAdapter(%q) = %v want %v", tc.in, got, tc.want)
		}
		if got.String() != tc.in {
			t.Errorf("roundtrip String() = %q want %q", got.String(), tc.in)
		}
	}
}

func TestAdapter_Parse_invalid_rejectsUppercase(t *testing.T) {
	t.Parallel()
	for _, in := range []string{""} {
		got, err := ParseAdapter(in)
		if err == nil {
			t.Errorf("ParseAdapter(%q) expected error, got %v", in, got)
			continue
		}
		if got != Adapter("") {
			t.Errorf("ParseAdapter(%q) adapter = %v want empty on failure", in, got)
		}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Errorf("ParseAdapter(%q) err %v does not match ErrInvalidAdapter", in, err)
		}
		var iae *InvalidAdapterError
		if !errors.As(err, &iae) {
			t.Errorf("ParseAdapter(%q) err %T is not *InvalidAdapterError", in, err)
			continue
		}
		if iae.Adapter != in {
			t.Errorf("ParseAdapter(%q) carried field = %q", in, iae.Adapter)
		}
	}
}
