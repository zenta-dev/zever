package scheduler

import (
	"errors"
	"testing"
)

func TestAdapterString(t *testing.T) {
	t.Parallel()

	if got := Embedded.String(); got != "embedded" {
		t.Fatalf("String()=%q want embedded", got)
	}

	if got := Adapter(999).String(); got != "unknown" {
		t.Fatalf("String()=%q want unknown", got)
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	a, err := ParseAdapter("embedded")
	if err != nil {
		t.Fatalf("ParseAdapter embedded: %v", err)
	}

	if a != Embedded {
		t.Fatalf("ParseAdapter=%v want Embedded", a)
	}

	for _, s := range []string{"", "Embedded", "EMBEDDED", "redis", "memory", " embedded"} {
		a, err := ParseAdapter(s)
		if err == nil {
			t.Fatalf("ParseAdapter %q want error", s)
		}

		if a != Embedded {
			t.Fatalf("ParseAdapter %q adapter=%v want Embedded", s, a)
		}

		var inv *InvalidAdapterError
		if !errors.As(err, &inv) {
			t.Fatalf("ParseAdapter %q err=%v want InvalidAdapterError", s, err)
		}

		if !errors.Is(err, ErrInvalidAdapter) {
			t.Fatalf("ParseAdapter %q err=%v want ErrInvalidAdapter", s, err)
		}
	}
}
