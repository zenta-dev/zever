package flag

// Adapter identifies the feature-flag backend implementation.
type Adapter int

const (
	// Static is the static file-backed flag adapter.
	Static Adapter = iota
	// Firebase is the Firebase-backed flag adapter.
	Firebase
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Static:
		return "static"
	case Firebase:
		return "firebase"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "static":
		return Static, nil
	case "firebase":
		return Firebase, nil
	default:
		return Static, &InvalidAdapterError{Adapter: s}
	}
}
