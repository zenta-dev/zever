package tenant

import (
	"context"
	"testing"
)

func TestContext_roundtrip(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTenant(context.Background(), "acme")
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatalf("FromContext ok = false, want true")
	}

	if got != "acme" {
		t.Errorf("FromContext = %q, want %q", got, "acme")
	}
}

func TestContext_missing_returnsEmptyFalse(t *testing.T) {
	t.Parallel()

	got, ok := FromContext(context.Background())
	if ok {
		t.Errorf("FromContext ok = true, want false")
	}

	if got != "" {
		t.Errorf("FromContext = %q, want empty", got)
	}
}

func TestContext_wrongType_returnsEmptyFalse(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), tenantKey{}, 123)
	got, ok := FromContext(ctx)
	if ok {
		t.Errorf("FromContext ok = true, want false")
	}

	if got != "" {
		t.Errorf("FromContext = %q, want empty", got)
	}
}

func TestContext_emptyID_present(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTenant(context.Background(), "")
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatalf("FromContext ok = false, want true for empty ID")
	}

	if got != "" {
		t.Errorf("FromContext = %q, want empty", got)
	}
}
