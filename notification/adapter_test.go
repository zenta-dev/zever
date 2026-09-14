package notification

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
		{Log, "log"},
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
	got, err := ParseAdapter("log")
	if err != nil {
		t.Fatalf("ParseAdapter(log) err = %v", err)
	}
	if got != Log {
		t.Fatalf("ParseAdapter(log) = %v want %v", got, Log)
	}
	if got.String() != "log" {
		t.Errorf("roundtrip String() = %q want %q", got.String(), "log")
	}
}

func TestAdapter_Parse_invalid_rejectsUppercase(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"LOG", "Log", "", "bogus", "log ", " log", "sms", "push", "SMTP"} {
		got, err := ParseAdapter(in)
		if err == nil {
			t.Errorf("ParseAdapter(%q) expected error, got %v", in, got)
			continue
		}
		if got != Log {
			t.Errorf("ParseAdapter(%q) adapter = %v want Log on failure", in, got)
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
