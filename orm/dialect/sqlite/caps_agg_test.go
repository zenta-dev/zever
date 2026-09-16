package sqlite

import "testing"

// TestSupportsAggregateFilter_version_gate pins the 3.30.0 floor.
func TestSupportsAggregateFilter_version_gate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "below floor", version: "3.29.5", want: false},
		{name: "at floor", version: "3.30.0", want: true},
		{name: "above floor", version: "3.46.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsAggregateFilter(); got != tt.want {
				t.Fatalf("SupportsAggregateFilter() on %s = %v, want %v", tt.version, got, tt.want)
			}
		})
	}

	if !New().SupportsAggregateFilter() {
		t.Fatal("New() must support aggregate FILTER (default 3.46.0)")
	}
}

// TestGroupingConstructs_always_false verifies ROLLUP/CUBE/GROUPING SETS
// stay false at every version.
func TestGroupingConstructs_always_false(t *testing.T) {
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

			if d.SupportsRollup() {
				t.Errorf("SupportsRollup() on %s = true, want false", tt.version)
			}

			if d.SupportsCube() {
				t.Errorf("SupportsCube() on %s = true, want false", tt.version)
			}

			if d.SupportsGroupingSets() {
				t.Errorf("SupportsGroupingSets() on %s = true, want false", tt.version)
			}
		})
	}
}
