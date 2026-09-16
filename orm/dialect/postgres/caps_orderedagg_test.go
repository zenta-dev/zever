package postgres

import "testing"

// TestOrderedAggregate_truth_table pins the aggregate-family answers.
func TestOrderedAggregate_truth_table(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsArrayAgg", got: d.SupportsArrayAgg(), want: true},
		{name: "SupportsStringAgg", got: d.SupportsStringAgg(), want: true},
		{name: "SupportsGroupConcat", got: d.SupportsGroupConcat(), want: false},
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
