package scheduler

import (
	"errors"
	"testing"
)

// TestEdgeOpenInvalidOptions fails closed before adapter lookup.
func TestEdgeOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := Open(benchAdapter(), Options{}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(invalid opts) err = %v, want ErrInvalidOptions", err)
	}
}
