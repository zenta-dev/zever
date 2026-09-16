package sqlite

import "testing"

// TestWindowFrame_version_gate pins the 3.28.0 floor for all three modes.
func TestWindowFrame_version_gate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "below floor", version: "3.27.2", want: false},
		{name: "at floor", version: "3.28.0", want: true},
		{name: "above floor", version: "3.46.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsWindowFrameRows(); got != tt.want {
				t.Errorf("SupportsWindowFrameRows() on %s = %v, want %v", tt.version, got, tt.want)
			}

			if got := d.SupportsWindowFrameRange(); got != tt.want {
				t.Errorf("SupportsWindowFrameRange() on %s = %v, want %v", tt.version, got, tt.want)
			}

			if got := d.SupportsWindowFrameGroups(); got != tt.want {
				t.Errorf("SupportsWindowFrameGroups() on %s = %v, want %v", tt.version, got, tt.want)
			}
		})
	}

	d := New()
	if !d.SupportsWindowFrameRows() || !d.SupportsWindowFrameRange() || !d.SupportsWindowFrameGroups() {
		t.Fatal("New() must support all window frame modes (default 3.46.0)")
	}
}
