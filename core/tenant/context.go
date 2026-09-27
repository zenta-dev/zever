package tenant

import (
	"context"
)

// tenantKey is the context key carrying the tenant ID.
type tenantKey struct{}

// ContextWithTenant returns a context carrying tenantID.
func ContextWithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// FromContext returns the tenant ID carried by ctx.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(tenantKey{}).(string)
	return id, ok
}
