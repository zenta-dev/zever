package log

// Adapter identifies a registered logging backend.

type Adapter string

const (
	// Noop selects the discard-all logging adapter.
	Noop Adapter = "noop"
	// ZeroLog selects the zerolog-backed logging adapter.
	ZeroLog Adapter = "zerolog"
	// Slog selects the standard library log/slog-backed logging adapter.
	Slog Adapter = "slog"
	// Pretty selects the human-readable local-development logging adapter.
	Pretty Adapter = "pretty"
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
