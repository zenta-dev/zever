package analytics

// Adapter identifies the analytics backend implementation.

type Adapter string

const (
	// Log selects the log analytics backend.
	Log Adapter = "log"
	// PostHog selects the PostHog analytics backend.
	PostHog Adapter = "posthog"
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
