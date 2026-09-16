package postgres

import "testing"

// TestSupportsLateral_always_true verifies LATERAL is unconditional.
func TestSupportsLateral_always_true(t *testing.T) {
	t.Parallel()

	if !New().SupportsLateral() {
		t.Fatal("New().SupportsLateral() = false, want true")
	}

	d, err := NewWithVersion("11.5")
	if err != nil {
		t.Fatalf("NewWithVersion(11.5): %v", err)
	}

	if !d.SupportsLateral() {
		t.Fatal("SupportsLateral() on 11.5 = false, want true")
	}
}
