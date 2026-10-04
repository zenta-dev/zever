package log

import (
	"errors"
	"testing"
)

func TestLevelString_maxValue_unknown(t *testing.T) {
	t.Parallel()

	if got := Level(255).String(); got != "unknown" {
		t.Fatalf("Level(255).String() = %q, want %q", got, "unknown")
	}
}

func TestParseAdapter_empty_returnsInvalidAdapterError(t *testing.T) {
	t.Parallel()

	a, err := ParseAdapter("")
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("ParseAdapter(\"\") err = %v, want ErrInvalidAdapter", err)
	}

	if a.String() != "unknown" {
		t.Fatalf("zero Adapter.String() = %q, want %q", a.String(), "unknown")
	}
}
