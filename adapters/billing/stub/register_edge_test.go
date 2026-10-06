package stub

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// TestRegisterAdapter proves Register wires Open into the billing registry:
// after registration the adapter kind resolves instead of reporting
// ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	b, err := billing.Open(billing.Stub, billing.Options{})
	if errors.Is(err, billing.ErrUnknownAdapter) {
		t.Fatalf("billing.Open(%q) after Register() = %v, want adapter wired", billing.Stub, err)
	}

	if b != nil {
		_ = b.Close()
	}
}
