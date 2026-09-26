package tenant

// Adapter identifies the tenant backend implementation.

type Adapter string

const (
	// Single selects the single-tenant backend.
	Single Adapter = "single"
	// Header selects the header-based tenant backend.
	Header Adapter = "header"
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
