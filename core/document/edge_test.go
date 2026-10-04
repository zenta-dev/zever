package document

import (
	"math"
	"testing"
)

func TestEdgeClampQuality_extremes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   int
		want int
	}{
		{"min int", math.MinInt, 0},
		{"negative", -1, 0},
		{"zero", 0, 0},
		{"mid", 50, 50},
		{"max", 100, 100},
		{"over", 101, 100},
		{"max int", math.MaxInt, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ClampQuality(tc.in); got != tc.want {
				t.Fatalf("ClampQuality(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestEdgeOptions_Validate_qualityBoundaries(t *testing.T) {
	t.Parallel()

	if err := (Options{Quality: 0, DPI: 0, MaxOutputBytes: 0, LatexRuns: 0}).Validate(); err != nil {
		t.Fatalf("zero boundaries err = %v, want nil", err)
	}

	if err := (Options{Quality: 100}).Validate(); err != nil {
		t.Fatalf("quality 100 err = %v, want nil", err)
	}

	if err := (Options{Quality: 101}).Validate(); err == nil {
		t.Fatal("quality 101 err = nil, want error")
	}
}
