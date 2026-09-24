package router

// Adapter identifies the HTTP router backend implementation.
type Adapter int

const (
	// AdapterFiber is the Fiber router adapter.
	AdapterFiber Adapter = iota
	// AdapterStdHTTP is the standard-library HTTP router adapter.
	AdapterStdHTTP
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case AdapterFiber:
		return "fiber"
	case AdapterStdHTTP:
		return "stdhttp"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "fiber":
		return AdapterFiber, nil
	case "stdhttp":
		return AdapterStdHTTP, nil
	default:
		return AdapterFiber, &InvalidAdapterError{Adapter: s}
	}
}
