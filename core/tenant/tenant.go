package tenant

import (
	"context"
)

// Tenant defines the multi-tenant resolution contract for tenant backends.
// Implementations return the zero value on error.
type Tenant interface {
	// Resolve returns the tenant ID for ctx and meta. It returns "" on error.
	Resolve(ctx context.Context, meta map[string]string) (string, error)
	// Scoped returns a context carrying tenantID. It returns nil on error.
	Scoped(ctx context.Context, tenantID string) (context.Context, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Tenant from the given Options.
type Factory func(opts Options) (Tenant, error)
