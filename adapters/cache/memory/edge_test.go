package memory

import (
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

// TestEdgeSet_emptyKeyAndValue covers the empty-key and zero-length-value
// boundary: both are valid and round-trip without error.
func TestEdgeSet_emptyKeyAndValue(t *testing.T) {
	t.Parallel()

	c := stubCache(t, cache.Options{})
	ctx := t.Context()

	if err := c.Set(ctx, "", nil, 0); err != nil {
		t.Fatalf("Set(empty) = %v, want nil", err)
	}

	got, err := c.Get(ctx, "")
	if err != nil {
		t.Fatalf("Get(empty) = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Fatalf("Get(empty) = %q, want empty", got)
	}
}

// TestRegisterOpensViaCoreOptions proves Register wires the adapter factory
// into the cache battery registry so cache.Open resolves it.
func TestRegisterOpensViaCoreOptions(t *testing.T) {
	Register()

	c, err := cache.Open(cache.Memory, cache.Options{})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	t.Cleanup(func() { _ = c.Close(t.Context()) })

	if setErr := c.Set(t.Context(), "k", []byte("v"), 0); setErr != nil {
		t.Fatalf("Set = %v", setErr)
	}
}
