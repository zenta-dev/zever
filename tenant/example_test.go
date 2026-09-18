package tenant_test

import (
	"github.com/zenta-dev/zever/tenant"
	tenantsingle "github.com/zenta-dev/zever/tenant/single"
)

// ExampleOpen opens the single-tenant backend with a fixed ID.
func ExampleOpen() {
	_ = tenant.Register(tenant.Single, tenantsingle.New)

	t, err := tenant.Open(tenant.Single, tenant.Options{ID: "default"})
	if err != nil {
		return
	}

	defer func() { _ = t.Close() }()
}
