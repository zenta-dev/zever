package sqlite

import "testing"

// TestQueryModifierCapabilities_all_false verifies DISTINCT ON, the
// Postgres-only lock modes, and TABLESAMPLE stay false.
func TestQueryModifierCapabilities_all_false(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
	}{
		{name: "SupportsDistinctOn", got: d.SupportsDistinctOn()},
		{name: "SupportsForNoKeyUpdate", got: d.SupportsForNoKeyUpdate()},
		{name: "SupportsForKeyShare", got: d.SupportsForKeyShare()},
		{name: "SupportsForUpdateOf", got: d.SupportsForUpdateOf()},
		{name: "SupportsTablesample", got: d.SupportsTablesample()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got {
				t.Fatalf("%s = true, want false", tt.name)
			}
		})
	}
}
