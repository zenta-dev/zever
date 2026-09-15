package analytics

// Adapter identifies the analytics backend implementation.
type Adapter int

const (
	// Log selects the log analytics backend.
	Log Adapter = iota
	// PostHog selects the PostHog analytics backend.
	PostHog
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Log:
		return "log"
	case PostHog:
		return "posthog"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "log":
		return Log, nil
	case "posthog":
		return PostHog, nil
	default:
		return Log, &InvalidAdapterError{Adapter: s}
	}
}
