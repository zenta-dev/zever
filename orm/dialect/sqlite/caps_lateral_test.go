package sqlite

import "testing"

// TestSupportsLateral_always_false verifies SQLite never reports LATERAL
// support at any version.
func TestSupportsLateral_always_false(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "old", version: "3.30.0"},
		{name: "current", version: "3.46.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if d.SupportsLateral() {
				t.Fatalf("SupportsLateral() on %s = true, want false", tt.version)
			}
		})
	}

	if New().SupportsLateral() {
		t.Fatal("New().SupportsLateral() = true, want false")
	}
}
