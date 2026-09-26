package flag

// Adapter identifies the feature-flag backend implementation.

type Adapter string

const (
	// Static is the static file-backed flag adapter.
	Static Adapter = "static"
	// Firebase is the Firebase-backed flag adapter.
	Firebase Adapter = "firebase"
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
