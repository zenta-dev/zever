package remote

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
)

// TestRegisterAdapter proves Register wires New into the i18n registry: after
// registration the adapter kind resolves instead of reporting
// ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	b, err := i18n.Open(i18n.Remote, i18n.Options{})
	if errors.Is(err, i18n.ErrUnknownAdapter) {
		t.Fatalf("i18n.Open(%q) after Register() = %v, want adapter wired", i18n.Remote, err)
	}

	if b != nil {
		_ = b.Close()
	}
}
