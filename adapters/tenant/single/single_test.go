package single

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

func TestResolveReturnsDefault(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	got, err := tn.Resolve(t.Context(), nil)
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil", err)
	}

	if got != tenant.DefaultSingleID {
		t.Fatalf("Resolve = %q, want %q", got, tenant.DefaultSingleID)
	}
}

func TestOpenCustomID(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{ID: "acme"})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	got, err := tn.Resolve(t.Context(), map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil", err)
	}

	if got != "acme" {
		t.Fatalf("Resolve = %q, want %q", got, "acme")
	}
}

func TestScopedSetsContext(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	ctx, err := tn.Scoped(t.Context(), "acme")
	if err != nil {
		t.Fatalf("Scoped err = %v, want nil", err)
	}

	id, ok := tenant.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext ok = false, want true")
	}

	if id != "acme" {
		t.Fatalf("FromContext = %q, want %q", id, "acme")
	}
}

func TestCloseNoop(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	if err := tn.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(tenant.Options{Header: "bad header"})
	if !errors.Is(err, tenant.ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenZeroOptsDefault(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}

	got, err := tn.Resolve(t.Context(), nil)
	if err != nil {
		t.Fatalf("Resolve err = %v, want nil", err)
	}

	if got != tenant.DefaultSingleID {
		t.Fatalf("Resolve = %q, want %q", got, tenant.DefaultSingleID)
	}
}
