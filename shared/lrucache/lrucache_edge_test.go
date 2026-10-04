package lrucache

import (
	"testing"
	"time"
)

// TestTTLCache_RangeNilCallback verifies a nil callback is a safe no-op.
func TestTTLCache_RangeNilCallback(t *testing.T) {
	t.Parallel()

	tc := NewTTL[string, int](4, time.Hour)
	tc.Put("a", 1)

	tc.Range(nil)

	if v, ok := tc.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = (%v, %v), want (1, true)", v, ok)
	}
}

// TestTTLCache_PurgeExpiredEmpty verifies sweeping an empty cache is a no-op.
func TestTTLCache_PurgeExpiredEmpty(t *testing.T) {
	t.Parallel()

	tc := NewTTL[string, int](4, time.Hour)
	if n := tc.PurgeExpired(); n != 0 {
		t.Fatalf("PurgeExpired() = %d, want 0", n)
	}
}
