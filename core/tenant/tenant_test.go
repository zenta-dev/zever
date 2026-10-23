package tenant

import (
	"context"
)

type stubTenant struct {
	id string
}

func (s *stubTenant) Resolve(_ context.Context, _ map[string]string) (string, error) {
	return s.id, nil
}

func (s *stubTenant) Scoped(ctx context.Context, tenantID string) (context.Context, error) {
	return ContextWithTenant(ctx, tenantID), nil
}

func (s *stubTenant) Close() error {
	return nil
}
