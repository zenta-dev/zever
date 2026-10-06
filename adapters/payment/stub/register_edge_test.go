package stub

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// TestRegisterAdapter proves Register wires New into the payment registry:
// after registration the adapter kind resolves instead of reporting
// ErrUnknownAdapter, and a repeated Register is harmless.
func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	p, err := payment.Open(payment.Stub, payment.Options{})
	if errors.Is(err, payment.ErrUnknownAdapter) {
		t.Fatalf("payment.Open(%q) after Register() = %v, want adapter wired", payment.Stub, err)
	}

	if p != nil {
		_ = p.Close()
	}
}
