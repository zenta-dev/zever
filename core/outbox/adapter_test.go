package outbox_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"memory", "db", "cdc", "custom"} {
		got, err := outbox.ParseAdapter(name)
		if err != nil {
			t.Errorf("ParseAdapter(%q) error = %v", name, err)
		}

		if string(got) != name {
			t.Errorf("ParseAdapter(%q) = %q", name, got)
		}
	}

	if _, err := outbox.ParseAdapter(""); !errors.Is(err, outbox.ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter(\"\") = %v, want ErrInvalidAdapter", err)
	}
}

func TestAdapterString(t *testing.T) {
	t.Parallel()

	if got := outbox.DB.String(); got != "db" {
		t.Errorf("DB.String() = %q, want db", got)
	}

	if got := outbox.Adapter("").String(); got != "unknown" {
		t.Errorf("empty String() = %q, want unknown", got)
	}
}
