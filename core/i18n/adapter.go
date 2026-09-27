package i18n

// Adapter identifies the internationalization backend implementation.

type Adapter string

const (
	// Embed is the embedded-catalog i18n adapter.
	Embed Adapter = "embed"
	// Remote is the remote-service i18n adapter.
	Remote Adapter = "remote"
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
