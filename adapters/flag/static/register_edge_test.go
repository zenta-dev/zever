package static

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// TestRegisterAdapter proves Register wires New into the flag registry: after
// registration the adapter kind resolves instead of reporting
// ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	f, err := flag.Open(flag.Static, flag.Options{})
	if errors.Is(err, flag.ErrUnknownAdapter) {
		t.Fatalf("flag.Open(%q) after Register() = %v, want adapter wired", flag.Static, err)
	}

	if f != nil {
		_ = f.Close()
	}
}
