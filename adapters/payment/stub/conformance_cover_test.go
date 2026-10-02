package stub

import (
	"testing"

	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/payment/paymenttest"
)

// TestStubConformance proves the stub adapter honors the
// payment.Payment lifecycle contract via the shared conformance kit.
// Each subtest gets a fresh in-memory ledger (no network, no funds).
// The stub never verifies webhooks, so the kit asserts the
// fail-closed webhook branch.
func TestStubConformance(t *testing.T) {
	paymenttest.Conformance(t, func(t *testing.T) payment.Payment {
		t.Helper()

		p, err := New(payment.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = p.Close() })

		return p
	})
}
