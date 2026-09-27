package router

// Adapter identifies the HTTP router backend implementation.

type Adapter string

const (
	// AdapterFiber is the Fiber router adapter.
	AdapterFiber Adapter = "fiber"
	// AdapterStdHTTP is the standard-library HTTP router adapter.
	AdapterStdHTTP Adapter = "stdhttp"
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
