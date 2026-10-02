package billingtest_test

import (
	"testing"

	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/billing/billingtest"
)

// TestConformanceStub proves the kit passes against the stub adapter.
func TestConformanceStub(t *testing.T) {
	t.Parallel()

	billingtest.Conformance(t, func(t *testing.T) billing.Billing {
		t.Helper()

		b, err := billingstub.Open(billing.Options{})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
