package i18n

// Adapter identifies the internationalization backend implementation.
type Adapter int

const (
	// Embed is the embedded-catalog i18n adapter.
	Embed Adapter = iota
	// Remote is the remote-service i18n adapter.
	Remote
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Embed:
		return "embed"
	case Remote:
		return "remote"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "embed":
		return Embed, nil
	case "remote":
		return Remote, nil
	default:
		return Embed, &InvalidAdapterError{Adapter: s}
	}
}
