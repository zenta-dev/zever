package permission

import (
	"errors"
	"testing"
)

func TestAdapter_String_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		adapter Adapter
		want    string
	}{
		{Noop, "noop"},
		{RBAC, "rbac"},
		{Casbin, "casbin"},
		{Adapter(999), "unknown"},
	}
	for _, c := range cases {
		if got := c.adapter.String(); got != c.want {
			t.Errorf("Adapter(%d).String() = %q want %q", int(c.adapter), got, c.want)
		}
	}
}

func TestAdapter_Parse_valid_roundtrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want Adapter
	}{
		{"noop", Noop},
		{"rbac", RBAC},
		{"casbin", Casbin},
	}
	for _, c := range cases {
		got, err := ParseAdapter(c.name)
		if err != nil {
			t.Errorf("ParseAdapter(%q) err = %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseAdapter(%q) = %v want %v", c.name, got, c.want)
		}
		if got.String() != c.name {
			t.Errorf("roundtrip String() = %q want %q", got.String(), c.name)
		}
	}
}

func TestAdapter_Parse_invalid(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"NOOP", "Rbac", "CASBIN", "", "bogus", "noop ", " rbac"} {
		got, err := ParseAdapter(in)
		if err == nil {
			t.Errorf("ParseAdapter(%q) expected error, got %v", in, got)
			continue
		}
		if got != Noop {
			t.Errorf("ParseAdapter(%q) adapter = %v want Noop on failure", in, got)
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
