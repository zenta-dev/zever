package postgres

import "testing"

// TestCTECapability_version_gates pins the 12.0 MATERIALIZED and 14.0
// SEARCH/CYCLE floors.
func TestCTECapability_version_gates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		version     string
		materialize bool
		searchCycle bool
	}{
		{name: "below materialize", version: "11.9", materialize: false, searchCycle: false},
		{name: "at materialize", version: "12.0", materialize: true, searchCycle: false},
		{name: "between floors", version: "13.4", materialize: true, searchCycle: false},
		{name: "at search cycle", version: "14.0", materialize: true, searchCycle: true},
		{name: "current", version: "16.0", materialize: true, searchCycle: true},
		{name: "future", version: "17.2", materialize: true, searchCycle: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsCTEMaterialized(); got != tt.materialize {
				t.Errorf("SupportsCTEMaterialized() on %s = %v, want %v",
					tt.version, got, tt.materialize)
			}

			if got := d.SupportsCTESearchCycle(); got != tt.searchCycle {
				t.Errorf("SupportsCTESearchCycle() on %s = %v, want %v",
					tt.version, got, tt.searchCycle)
			}
		})
	}

	if !New().SupportsCTEMaterialized() || !New().SupportsCTESearchCycle() {
		t.Fatal("New() (defaults to 16.0) must report both CTE extensions")
	}
}
