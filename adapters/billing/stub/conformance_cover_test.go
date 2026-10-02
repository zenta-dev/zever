package stub

import (
	"testing"

	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/billing/billingtest"
)

// TestStubConformance proves the stub adapter honors the
// billing.Billing lifecycle contract via the shared conformance kit.
// Each subtest gets a fresh in-memory backend (no network). The stub
// ignores idempotency keys and mints zero-amount invoices; the kit
// asserts lifecycle linkage and sentinels only.
func TestStubConformance(t *testing.T) {
	billingtest.Conformance(t, func(t *testing.T) billing.Billing {
		t.Helper()

		b, err := Open(billing.Options{})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
