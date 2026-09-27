package mailer

// Adapter identifies the mailer backend implementation.

type Adapter string

const (
	// Log is the log mailer adapter.
	Log Adapter = "log"
	// SMTP is the SMTP mailer adapter.
	SMTP Adapter = "smtp"
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
