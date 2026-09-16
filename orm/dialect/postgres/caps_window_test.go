package postgres

import "testing"

// TestWindowFrame_all_true verifies all three frame modes are
// unconditional on Postgres.
func TestWindowFrame_all_true(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
	}{
		{name: "SupportsWindowFrameRows", got: d.SupportsWindowFrameRows()},
		{name: "SupportsWindowFrameRange", got: d.SupportsWindowFrameRange()},
		{name: "SupportsWindowFrameGroups", got: d.SupportsWindowFrameGroups()},
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
