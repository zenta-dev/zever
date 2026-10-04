package header

import (
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
