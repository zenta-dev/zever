package geo

import (
	"math"
	"testing"
)

func TestEdgeValidCoord_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lat  float64
		lng  float64
		want bool
	}{
		{"origin", 0, 0, true},
		{"north pole", 90, 0, true},
		{"south pole", -90, 0, true},
		{"east edge", 0, 180, true},
		{"west edge", 0, -180, true},
		{"lat over", 90.0001, 0, false},
		{"lat under", -90.0001, 0, false},
		{"lng over", 0, 180.0001, false},
		{"lng under", 0, -180.0001, false},
		{"nan lat", math.NaN(), 0, false},
		{"inf lng", 0, math.Inf(1), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ValidCoord(tc.lat, tc.lng); got != tc.want {
				t.Fatalf("ValidCoord(%v, %v) = %v, want %v", tc.lat, tc.lng, got, tc.want)
			}
		})
	}
}

func TestEdgeOptions_Validate_negativeValues(t *testing.T) {
	t.Parallel()

	if err := (Options{Timeout: -1}).Validate(); err == nil {
		t.Fatal("negative Timeout err = nil, want error")
	}

	if err := (Options{MaxResponseBody: -1}).Validate(); err == nil {
		t.Fatal("negative MaxResponseBody err = nil, want error")
	}
}
