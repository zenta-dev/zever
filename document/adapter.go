package document

// Adapter identifies the document backend implementation.
type Adapter int

const (
	// Local selects the local document backend.
	Local Adapter = iota
	// Remote selects the remote document backend.
	Remote
	// Latex selects the LaTeX document backend.
	Latex
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Local:
		return "local"
	case Remote:
		return "remote"
	case Latex:
		return "latex"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "local":
		return Local, nil
	case "remote":
		return Remote, nil
	case "latex":
		return Latex, nil
	default:
		return Local, &InvalidAdapterError{Adapter: s}
	}
}
