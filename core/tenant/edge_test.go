package tenant

import (
	"testing"
)

func TestOptions_Validate_id_anyValue(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"", "acme", "a/b", "UPPER-123"} {
		if err := (Options{ID: id}).Validate(); err != nil {
			t.Fatalf("Validate(ID=%q) err = %v, want nil", id, err)
		}
	}
}

func TestContext_nested_overwrite(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTenant(t.Context(), "first")
	ctx = ContextWithTenant(ctx, "second")

	got, ok := FromContext(ctx)
	if !ok || got != "second" {
		t.Fatalf("FromContext = %q,%v want second,true", got, ok)
	}
}

func TestContext_nilParentValue(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTenant(t.Context(), "acme")
	if ctx == nil {
		t.Fatal("ContextWithTenant returned nil")
	}
}
