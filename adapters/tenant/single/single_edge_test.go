package single

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

// TestResolveConcurrent exercises the fixed-ID Resolve path from many
// goroutines; the adapter is immutable and must be safe to share.
func TestResolveConcurrent(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{ID: "acme"})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = tn.Close() })

	ctx := t.Context()
	meta := map[string]string{"x": "y"}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			id, rerr := tn.Resolve(ctx, meta)
			if rerr != nil {
				t.Errorf("Resolve() err = %v, want nil", rerr)
				return
			}

			if id != "acme" {
				t.Errorf("Resolve() = %q, want %q", id, "acme")
			}
		}()
	}

	wg.Wait()
}

// TestScopedConcurrent exercises context scoping from many goroutines; each
// call derives an independent context value.
func TestScopedConcurrent(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = tn.Close() })

	ctx := t.Context()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			scoped, serr := tn.Scoped(ctx, "acme")
			if serr != nil {
				t.Errorf("Scoped() err = %v, want nil", serr)
				return
			}

			if id, ok := tenant.FromContext(scoped); !ok || id != "acme" {
				t.Errorf("FromContext() = (%q, %v), want (%q, true)", id, ok, "acme")
			}
		}()
	}

	wg.Wait()
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	if err := tn.Close(); err != nil {
		t.Fatalf("Close #1 err = %v, want nil", err)
	}
	if err := tn.Close(); err != nil {
		t.Fatalf("Close #2 err = %v, want nil", err)
	}
}

// TestEdgeScoped_emptyID proves Scoped accepts an empty tenant ID without
// error; the fixed-ID adapter performs no validation.
func TestEdgeScoped_emptyID(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	ctx, err := tn.Scoped(t.Context(), "")
	if err != nil {
		t.Fatalf("Scoped(\"\") err = %v, want nil", err)
	}
	if got, ok := tenant.FromContext(ctx); !ok || got != "" {
		t.Fatalf("FromContext() = (%q, %v), want (%q, true)", got, ok, "")
	}
}

// TestEdgeResolve_ignoresMeta proves Resolve returns the fixed ID regardless
// of metadata content, including nil metadata.
func TestEdgeResolve_ignoresMeta(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{ID: "fixed"})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	for _, meta := range []map[string]string{nil, {}, {"X-Tenant-ID": "other"}} {
		got, rerr := tn.Resolve(t.Context(), meta)
		if rerr != nil {
			t.Fatalf("Resolve(%v) err = %v, want nil", meta, rerr)
		}
		if got != "fixed" {
			t.Fatalf("Resolve(%v) = %q, want %q", meta, got, "fixed")
		}
	}
}
