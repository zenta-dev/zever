package noop

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	c, err := cdn.Open(cdn.AdapterNoop, cdn.Options{})
	if errors.Is(err, cdn.ErrUnknownAdapter) {
		t.Fatalf("cdn.Open(%q) after Register() = %v, want adapter wired", cdn.AdapterNoop, err)
	}
	if err != nil {
		t.Fatalf("cdn.Open() error = %v", err)
	}
	if c != nil {
		_ = c.Close(t.Context())
	}
}
