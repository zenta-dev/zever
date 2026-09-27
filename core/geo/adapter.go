package geo

// Adapter identifies the geo backend implementation.

type Adapter string

const (
	// Google selects the Google Maps geo backend.
	Google Adapter = "google"
	// Static selects the local JSON-backed geo backend.
	Static Adapter = "static"
	// OSM selects the OpenStreetMap Nominatim geo backend.
	OSM Adapter = "osm"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), &InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
