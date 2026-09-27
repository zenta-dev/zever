package geo

import (
	"math"
	"testing"
)

func TestValidCoord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lat  float64
		lng  float64
		want bool
	}{
		{name: "origin", lat: 0, lng: 0, want: true},
		{name: "max corner", lat: 90, lng: 180, want: true},
		{name: "min corner", lat: -90, lng: -180, want: true},
		{name: "lat too high", lat: 90.1, lng: 0, want: false},
		{name: "lat too low", lat: -90.1, lng: 0, want: false},
		{name: "lng too high", lat: 0, lng: 180.1, want: false},
		{name: "lng too low", lat: 0, lng: -180.1, want: false},
		{name: "NaN lat", lat: math.NaN(), lng: 0, want: false},
		{name: "NaN lng", lat: 0, lng: math.NaN(), want: false},
		{name: "Inf lat", lat: math.Inf(1), lng: 0, want: false},
		{name: "-Inf lat", lat: math.Inf(-1), lng: 0, want: false},
		{name: "Inf lng", lat: 0, lng: math.Inf(1), want: false},
		{name: "-Inf lng", lat: 0, lng: math.Inf(-1), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidCoord(tt.lat, tt.lng); got != tt.want {
				t.Fatalf("ValidCoord(%v, %v) = %v, want %v", tt.lat, tt.lng, got, tt.want)
			}
		})
	}
}

func TestZeroValues(t *testing.T) {
	t.Parallel()

	t.Run("zero values", func(t *testing.T) {
		t.Parallel()
		var loc Location
		if loc.Lat != 0 || loc.Lng != 0 || loc.Formatted != "" {
			t.Fatalf("zero Location = %+v", loc)
		}
		var addr Address
		if addr.Formatted != "" || addr.Components != nil {
			t.Fatalf("zero Address = %+v", addr)
		}
		var p Point
		if p.Lat != 0 || p.Lng != 0 {
			t.Fatalf("zero Point = %+v", p)
		}
		if !ValidCoord(p.Lat, p.Lng) {
			t.Fatal("zero Point should be valid coords")
		}
	})
}
