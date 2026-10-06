package header

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
)

// TestResolveConcurrent exercises both the header and subdomain resolution
// paths from many goroutines; the adapter holds only immutable compiled state.
func TestResolveConcurrent(t *testing.T) {
	t.Parallel()

	tn, err := New(tenant.Options{
		Header:         "X-Tenant-ID",
		SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`,
	})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	t.Cleanup(func() { _ = tn.Close() })

	ctx := t.Context()

	metas := []map[string]string{
		{"X-Tenant-ID": "acme"},
		{"Host": "acme.example.com:8080"},
	}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			id, rerr := tn.Resolve(ctx, metas[i%len(metas)])
			if rerr != nil {
				t.Errorf("Resolve() err = %v, want nil", rerr)
				return
			}

			if id != "acme" {
				t.Errorf("Resolve() = %q, want %q", id, "acme")
			}
		}(i)
	}

	wg.Wait()
}

// TestEdgeResolve_emptyHeaderValueSkipped proves an empty tenant-header value
// is treated as absent: resolution falls through to the subdomain, and with no
// subdomain configured reports ErrNotFound.
func TestEdgeResolve_emptyHeaderValueSkipped(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)

	withSub := &adapter{header: "X-Tenant-ID", subdomainRe: re, subdomainFmt: `^([a-z0-9-]+)\.example\.com$`}
	id, err := withSub.Resolve(t.Context(), map[string]string{
		"X-Tenant-ID": "",
		"Host":        "acme.example.com",
	})
	if err != nil {
		t.Fatalf("Resolve(empty header + subdomain) err = %v, want nil", err)
	}
	if id != "acme" {
		t.Fatalf("Resolve = %q, want acme (subdomain fallback)", id)
	}

	noSub := &adapter{header: "X-Tenant-ID"}
	if _, err := noSub.Resolve(t.Context(), map[string]string{"X-Tenant-ID": ""}); !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("Resolve(empty header, no subdomain) err = %v, want ErrNotFound", err)
	}
}

// TestEdgeScoped_emptyID proves Scoped accepts an empty tenant ID without
// error; validation of the ID is the resolver's job, not Scoped's.
func TestEdgeScoped_emptyID(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}

	ctx, err := a.Scoped(t.Context(), "")
	if err != nil {
		t.Fatalf("Scoped(\"\") err = %v, want nil", err)
	}
	if got, ok := tenant.FromContext(ctx); !ok || got != "" {
		t.Fatalf("FromContext = (%q, %v), want (\"\", true)", got, ok)
	}
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}

	if err := a.Close(); err != nil {
		t.Fatalf("Close #1 err = %v, want nil", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close #2 err = %v, want nil", err)
	}
}

// TestEdgeNew_invalidOptionsWithPattern proves core option validation runs
// before the subdomain pattern is compiled.
func TestEdgeNew_invalidOptionsWithPattern(t *testing.T) {
	t.Parallel()

	_, err := New(tenant.Options{Header: "bad header", SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`})
	if !errors.Is(err, tenant.ErrInvalidOptions) {
		t.Fatalf("New err = %v, want ErrInvalidOptions", err)
	}
}
