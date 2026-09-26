package geo

import "math"

// Location represents a geocoded point.
type Location struct {
	// Lat is latitude in degrees, -90..90.
	Lat float64
	// Lng is longitude in degrees, -180..180.
	Lng float64
	// Formatted is the human-readable place name.
	Formatted string
}

// Address represents a reverse-geocoded result.
type Address struct {
	// Formatted is the human-readable address.
	Formatted string
	// Components holds address parts keyed by type (e.g., locality, country).
	Components map[string]string
}

// Point is a WGS84 coordinate pair.
type Point struct {
	// Lat is latitude in degrees.
	Lat float64
	// Lng is longitude in degrees.
	Lng float64
}

// ValidCoord reports whether lat/lng are in WGS84 range and finite.
func ValidCoord(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) || math.IsInf(lat, 0) || math.IsInf(lng, 0) {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
