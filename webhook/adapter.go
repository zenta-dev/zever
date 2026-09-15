package webhook

// Adapter identifies the webhook backend implementation.
type Adapter int

const (
	// AdapterHTTP delivers webhooks over HTTPS with retries.
	AdapterHTTP Adapter = iota
	// AdapterQueue delivers webhooks through a queue topic.
	AdapterQueue
	// AdapterSQLite persists webhook subscriptions in SQLite.
	AdapterSQLite
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case AdapterHTTP:
		return "http"
	case AdapterQueue:
		return "queue"
	case AdapterSQLite:
		return "sqlite"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "http":
		return AdapterHTTP, nil
	case "queue":
		return AdapterQueue, nil
	case "sqlite":
		return AdapterSQLite, nil
	default:
		return AdapterHTTP, &InvalidAdapterError{Adapter: s}
	}
}
