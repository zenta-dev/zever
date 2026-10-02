package tenanttest_test

import (
	"testing"

	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/tenant/tenanttest"
)

// TestConformanceSingle proves the kit passes against the single adapter.
func TestConformanceSingle(t *testing.T) {
	t.Parallel()

	tenanttest.Conformance(t, func(t *testing.T) tenant.Tenant {
		t.Helper()

		b, err := tenantsingle.New(tenant.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
