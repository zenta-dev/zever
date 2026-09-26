package document

// Adapter identifies the document backend implementation.

type Adapter string

const (
	// Local selects the local document backend.
	Local Adapter = "local"
	// Remote selects the remote document backend.
	Remote Adapter = "remote"
	// Latex selects the LaTeX document backend.
	Latex Adapter = "latex"
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
