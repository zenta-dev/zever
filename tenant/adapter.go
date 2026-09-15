package tenant

// Adapter identifies the tenant backend implementation.
type Adapter int

const (
	// Single selects the single-tenant backend.
	Single Adapter = iota
	// Header selects the header-based tenant backend.
	Header
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Single:
		return "single"
	case Header:
		return "header"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "single":
		return Single, nil
	case "header":
		return Header, nil
	default:
		return Single, &InvalidAdapterError{Adapter: s}
	}
}
