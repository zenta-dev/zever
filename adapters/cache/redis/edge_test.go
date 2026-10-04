package redis

import "testing"

// TestEdgeEmptyValue_roundTrip covers the zero-length-value boundary against
// an in-process miniredis server. Sequential by design: New threads through
// the shared client singleton, so t.Parallel is forbidden here.
func TestEdgeEmptyValue_roundTrip(t *testing.T) {
	a, _ := newLiveAdapter(t)

	ctx := t.Context()

	if err := a.Set(ctx, "empty", nil, 0); err != nil {
		t.Fatalf("Set(nil) = %v, want nil", err)
	}

	got, err := a.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Fatalf("Get = %q, want empty", got)
	}
}
