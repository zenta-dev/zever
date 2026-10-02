// Package tenanttest provides the conformance kit third-party tenant adapters run to prove backend parity.
package tenanttest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

// Conformance verifies factory-built backends implement the
// tenant.Tenant contract: open/register round-trip, fixture-meta
// resolution to a non-empty ID, Scoped/FromContext round-trip, and
// Close. Each subtest takes a fresh instance from factory so cases
// stay isolated. Tests never call time.Sleep and never touch the
// network.
//
// Fixture contract: the factory must be configured so Resolve with
// meta {"X-Tenant-ID": "kit-tenant"} succeeds. Header-shaped backends
// return "kit-tenant"; fixed-ID backends (single) return their
// configured ID. The kit asserts non-empty success rather than an
// exact ID so one kit covers both shapes; exact-ID coverage stays
// adapter-owned.
func Conformance(t *testing.T, factory func(t *testing.T) tenant.Tenant) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Resolve", func(t *testing.T) { conformanceResolve(t, factory) })
	t.Run("Scoped", func(t *testing.T) { conformanceScoped(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := tenant.Open(tenant.Adapter("conformance-missing-adapter"), tenant.Options{}); !errors.Is(err, tenant.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := tenant.Adapter("conformance-probe-tenant")

	if err := tenant.Register(probe, nil); !errors.Is(err, tenant.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(tenant.Options) (tenant.Tenant, error) {
		return nil, errors.New("tenanttest: probe factory must not run")
	}

	_ = tenant.Register(probe, stub)

	if err := tenant.Register(probe, stub); !errors.Is(err, tenant.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceResolve(t *testing.T, factory func(t *testing.T) tenant.Tenant) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	got, err := b.Resolve(ctx, map[string]string{"X-Tenant-ID": "kit-tenant"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got == "" {
		t.Error("Resolve() = empty, want non-empty tenant ID")
	}
}

func conformanceScoped(t *testing.T, factory func(t *testing.T) tenant.Tenant) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	scoped, err := b.Scoped(ctx, "kit-tenant")
	if err != nil {
		t.Fatalf("Scoped() error = %v", err)
	}

	if scoped == nil {
		t.Fatal("Scoped() context is nil")
	}

	got, ok := tenant.FromContext(scoped)
	if !ok {
		t.Fatal("FromContext() = false, want carried tenant")
	}

	if got != "kit-tenant" {
		t.Errorf("FromContext() = %q, want kit-tenant", got)
	}

	if _, ok := tenant.FromContext(ctx); ok {
		t.Error("FromContext(bare) = true, want false")
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) tenant.Tenant) {
	t.Helper()

	b := factory(t)

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
