package sqlite

import "testing"

// TestOrderedAggregate_truth_table pins the aggregate-family answers on
// the default version.
func TestOrderedAggregate_truth_table(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsArrayAgg", got: d.SupportsArrayAgg(), want: false},
		{name: "SupportsStringAgg", got: d.SupportsStringAgg(), want: false},
		{name: "SupportsGroupConcat", got: d.SupportsGroupConcat(), want: true},
		{name: "SupportsOrderedAggregates", got: d.SupportsOrderedAggregates(), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestSupportsOrderedAggregates_version_gate pins the 3.44.0 floor.
func TestSupportsOrderedAggregates_version_gate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "below floor", version: "3.43.2", want: false},
		{name: "at floor", version: "3.44.0", want: true},
		{name: "above floor", version: "3.46.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsOrderedAggregates(); got != tt.want {
				t.Fatalf("SupportsOrderedAggregates() on %s = %v, want %v", tt.version, got, tt.want)
			}

			if d.SupportsArrayAgg() || d.SupportsStringAgg() {
				t.Fatalf("array_agg/string_agg on %s must stay false", tt.version)
			}

			if !d.SupportsGroupConcat() {
				t.Fatalf("group_concat on %s must stay true", tt.version)
			}
		})
	}
}
