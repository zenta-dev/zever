package idempotency

import (
	"testing"
)

// TestEdgeInvalidAdapterErrorString covers InvalidAdapterError.Error.
func TestEdgeInvalidAdapterErrorString(t *testing.T) {
	t.Parallel()

	err := InvalidAdapterError{Adapter: "bogus"}
	want := `idempotency: invalid adapter: "bogus"`
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
