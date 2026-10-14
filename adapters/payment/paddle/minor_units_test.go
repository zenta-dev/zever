package paddle

import (
	"math"
	"testing"
)

// TestPaddleMinorUnitsRoundTrip pins refund symmetry at the codec level:
// every minor-unit total formats to a major-unit decimal that parses back
// to itself, so parsed totals and formatted adjustments always agree.
func TestPaddleMinorUnitsRoundTrip(t *testing.T) {
	t.Parallel()

	values := []int64{
		0, 1, -1, 50, -50, 500, 1000, 2500, 9999, 10000,
		math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1,
	}
	for _, v := range values {
		enc := formatPaddleAmount(v)

		got, err := parsePaddleAmount(enc)
		if err != nil {
			t.Errorf("round-trip(%d) via %q err = %v", v, enc, err)
			continue
		}

		if got != v {
			t.Errorf("round-trip(%d) = %d via %q", v, got, enc)
		}
	}
}

// TestFormatPaddleAmount_table pins the outbound major-unit encoding used
// for partial-refund adjustments.
func TestFormatPaddleAmount_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value int64
		want  string
	}{
		{0, "0.00"},
		{1, "0.01"},
		{1000, "10.00"},
		{-525, "-5.25"},
		{math.MaxInt64, "92233720368547758.07"},
		{math.MinInt64, "-92233720368547758.08"},
	}
	for _, tc := range cases {
		if got := formatPaddleAmount(tc.value); got != tc.want {
			t.Errorf("formatPaddleAmount(%d) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
