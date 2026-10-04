package tenant

import (
	"errors"
	"sync"
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

func TestOpen_invalidHeader_colon(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := Register(a, func(Options) (Tenant, error) { return &stubTenant{}, nil }); err != nil {
		t.Fatalf("Register(%v) error = %v", a, err)
	}

	_, err := Open(a, Options{Header: "X:Bad"})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(colon header) err = %v, want ErrInvalidOptions", err)
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := Register(a, func(Options) (Tenant, error) { return &stubTenant{id: "acme"}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
