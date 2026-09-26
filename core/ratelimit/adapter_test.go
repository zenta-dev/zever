package ratelimit

import (
	"errors"
	"testing"
)

func TestAdapter_String_roundtrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		adapter Adapter
		want    string
	}{
		{Memory, "memory"},
		{Redis, "redis"},
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
	cases := []struct {
		name string
		want Adapter
	}{
		{"memory", Memory},
		{"redis", Redis},
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
