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
	default:
		return "unknown"
	}
}

// ParseAdapter converts a canonical adapter name into an Adapter.
func ParseAdapter(adapter string) (Adapter, error) {
	switch adapter {
	case "noop":
		return Noop, nil
	case "zerolog":
		return ZeroLog, nil
	case "slog":
		return Slog, nil
	default:
		return Noop, &InvalidAdapterError{Adapter: adapter}
	}
}
