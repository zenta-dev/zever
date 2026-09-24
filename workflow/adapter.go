package workflow

// Adapter identifies the workflow backend implementation.
type Adapter int

const (
	// Memory is the in-memory workflow adapter.
	Memory Adapter = iota
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Memory:
		return "memory"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// It returns InvalidAdapterError for unknown names.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "memory":
		return Memory, nil
	default:
		return Memory, &InvalidAdapterError{Adapter: s}
	}
}
