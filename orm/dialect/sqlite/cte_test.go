package sqlite

import "testing"

// TestSupportsCTEMaterialized_version_gate pins the 3.35.0 floor.
func TestSupportsCTEMaterialized_version_gate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "below floor", version: "3.34.9", want: false},
		{name: "at floor", version: "3.35.0", want: true},
		{name: "above floor", version: "3.46.0", want: true},
		{name: "future major", version: "4.0.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsCTEMaterialized(); got != tt.want {
				t.Fatalf("SupportsCTEMaterialized() on %s = %v, want %v", tt.version, got, tt.want)
			}
		})
	}

	if !New().SupportsCTEMaterialized() {
		t.Fatal("New() must support CTE MATERIALIZED (default 3.46.0)")
	}
}

// TestSupportsCTESearchCycle_always_false verifies SQLite never reports
// SEARCH/CYCLE support at any version.
func TestSupportsCTESearchCycle_always_false(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "old", version: "3.30.0"},
		{name: "current", version: "3.46.0"},
		{name: "future", version: "4.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if d.SupportsCTESearchCycle() {
				t.Fatalf("SupportsCTESearchCycle() on %s = true, want false", tt.version)
			}
		})
	}

	if New().SupportsCTESearchCycle() {
		t.Fatal("New().SupportsCTESearchCycle() = true, want false")
	}
}
