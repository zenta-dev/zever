package postgres

import "testing"

// TestAggregateGrouping_all_true verifies Postgres supports the full
// aggregate/grouping surface.
func TestAggregateGrouping_all_true(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
	}{
		{name: "SupportsAggregateFilter", got: d.SupportsAggregateFilter()},
		{name: "SupportsRollup", got: d.SupportsRollup()},
		{name: "SupportsCube", got: d.SupportsCube()},
		{name: "SupportsGroupingSets", got: d.SupportsGroupingSets()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.got {
				t.Fatalf("%s = false, want true", tt.name)
			}
		})
	}
}
