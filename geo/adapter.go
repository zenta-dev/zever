package geo

// Adapter identifies the geo backend implementation.
type Adapter int

const (
	// Google selects the Google Maps geo backend.
	Google Adapter = iota
	// Static selects the local JSON-backed geo backend.
	Static
	// OSM selects the OpenStreetMap Nominatim geo backend.
	OSM
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Google:
		return "google"
	case Static:
		return "static"
	case OSM:
		return "osm"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "google":
		return Google, nil
	case "static":
		return Static, nil
	case "osm":
		return OSM, nil
	default:
		return Google, &InvalidAdapterError{Adapter: s}
	}
}
