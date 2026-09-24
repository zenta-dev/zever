package log

// Adapter identifies a registered logging backend.
type Adapter int

const (
	// Noop selects the discard-all logging adapter.
	Noop Adapter = iota
	// ZeroLog selects the zerolog-backed logging adapter.
	ZeroLog
	// Slog selects the standard library log/slog-backed logging adapter.
	Slog
	// Pretty selects the human-readable local-development logging adapter.
	Pretty
)

// String returns the canonical lowercase name of the adapter.
func (a Adapter) String() string {
	switch a {
	case Noop:
		return "noop"
	case ZeroLog:
		return "zerolog"
	case Slog:
		return "slog"
	case Pretty:
		return "pretty"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "noop":
		return Noop, nil
	case "zerolog":
		return ZeroLog, nil
	case "slog":
		return Slog, nil
	case "pretty":
		return Pretty, nil
	default:
		return Noop, &InvalidAdapterError{Adapter: s}
	}
}
