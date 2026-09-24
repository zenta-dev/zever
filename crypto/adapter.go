package crypto

// Adapter identifies the cryptography backend implementation.
type Adapter int

const (
	// AdapterLocal is the local cryptography adapter.
	AdapterLocal Adapter = iota
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case AdapterLocal:
		return "local"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "local":
		return AdapterLocal, nil
	default:
		return AdapterLocal, &InvalidAdapterError{Adapter: s}
	}
}
