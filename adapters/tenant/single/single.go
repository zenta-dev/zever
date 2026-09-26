package single

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/tenant"
)

// adapter is the fixed-ID single-tenant backend.
type adapter struct {
	// id is the tenant ID always returned by Resolve.
	id string
}

// Resolve returns the fixed tenant ID, ignoring context and metadata.
func (a *adapter) Resolve(_ context.Context, _ map[string]string) (string, error) {
	return a.id, nil
}

// Scoped returns a context carrying the given tenant ID.
func (a *adapter) Scoped(ctx context.Context, tenantID string) (context.Context, error) {
	return tenant.ContextWithTenant(ctx, tenantID), nil
}

// Close releases backend resources, of which single holds none.
func (a *adapter) Close() error {
	return nil
}

// New creates a fixed-ID Tenant, defaulting empty ID to DefaultSingleID.
func New(o tenant.Options) (tenant.Tenant, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("single: %w", err)
	}

	id := o.ID
	if id == "" {
		id = tenant.DefaultSingleID
	}

	return &adapter{id: id}, nil
}
