package paymenttest_test

import (
	"testing"

	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/payment/paymenttest"
)

// TestConformanceStub proves the kit passes against the stub adapter.
func TestConformanceStub(t *testing.T) {
	t.Parallel()

	paymenttest.Conformance(t, func(t *testing.T) payment.Payment {
		t.Helper()

		p, err := paymentstub.New(payment.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = p.Close() })

		return p
	})
}
