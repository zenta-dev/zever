package static

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// TestRegisterAdapter proves Register wires New into the geo registry: after
// registration the adapter kind resolves instead of reporting
// ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	g, err := geo.Open(geo.Static, geo.Options{})
	if errors.Is(err, geo.ErrUnknownAdapter) {
		t.Fatalf("geo.Open(%q) after Register() = %v, want adapter wired", geo.Static, err)
	}

	if g != nil {
		_ = g.Close()
	}
}
