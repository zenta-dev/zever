package scheduler

// Adapter identifies the scheduler backend implementation.
type Adapter int

const (
	// Embedded is the in-process cron scheduler adapter.
	Embedded Adapter = iota
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Embedded:
		return "embedded"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "embedded":
		return Embedded, nil
	default:
		return Embedded, &InvalidAdapterError{Adapter: s}
	}
}
