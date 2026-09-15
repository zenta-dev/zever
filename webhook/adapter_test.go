package webhook

import (
	"errors"
	"testing"
)

func TestAdapter_String_returnsName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		adapter Adapter
		want    string
	}{
		{"http", AdapterHTTP, "http"},
		{"queue", AdapterQueue, "queue"},
		{"sqlite", AdapterSQLite, "sqlite"},
		{"unknown", Adapter(999), "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.adapter.String(); got != c.want {
				t.Errorf("Adapter(%d).String() = %q want %q", int(c.adapter), got, c.want)
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
		{"http", AdapterHTTP},
		{"queue", AdapterQueue},
		{"sqlite", AdapterSQLite},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAdapter(c.name)
			if err != nil {
				t.Fatalf("ParseAdapter(%q) err = %v", c.name, err)
			}
			if got != c.want {
				t.Fatalf("ParseAdapter(%q) = %v want %v", c.name, got, c.want)
			}
			if got.String() != c.name {
				t.Errorf("roundtrip String() = %q want %q", got.String(), c.name)
			}
		})
	}
}

func TestAdapter_Parse_invalid(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"HTTP", "Http", "", "bogus", "http ", " http", "postgres", "SMTP"} {
		t.Run("input:"+in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAdapter(in)
			if err == nil {
				t.Fatalf("ParseAdapter(%q) expected error, got %v", in, got)
			}
			if got != AdapterHTTP {
				t.Errorf("ParseAdapter(%q) adapter = %v want AdapterHTTP on failure", in, got)
			}
			if !errors.Is(err, ErrInvalidAdapter) {
				t.Errorf("ParseAdapter(%q) err %v does not match ErrInvalidAdapter", in, err)
			}
			var iae *InvalidAdapterError
			if !errors.As(err, &iae) {
				t.Fatalf("ParseAdapter(%q) err %T is not *InvalidAdapterError", in, err)
			}
			if iae.Adapter != in {
				t.Errorf("ParseAdapter(%q) carried field = %q", in, iae.Adapter)
			}
		})
	}
}
