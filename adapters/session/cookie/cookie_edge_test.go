package cookie

import (
	"strings"
	"testing"
)

// TestNew_valueBoundaries covers empty and very large session IDs: the value
// is passed through verbatim with no truncation.
func TestNew_valueBoundaries(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 1<<20)

	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"single", "x"},
		{"long", long},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(tt.value, Options{})
			if c.Value != tt.value {
				t.Fatalf("Value length = %d, want %d", len(c.Value), len(tt.value))
			}
		})
	}
}

// TestNew_emptyNameAndPath_defaulted verifies an explicitly empty Name/Path
// still falls back to the secure defaults rather than emitting blanks.
func TestNew_emptyNameAndPath_defaulted(t *testing.T) {
	t.Parallel()

	c := New("sess-id", Options{Name: "", Path: ""})

	if c.Name != DefaultName {
		t.Fatalf("Name = %q, want %q", c.Name, DefaultName)
	}

	if c.Path != DefaultPath {
		t.Fatalf("Path = %q, want %q", c.Path, DefaultPath)
	}
}
