package mailer

// Adapter identifies the mailer backend implementation.
type Adapter int

const (
	// Log is the log mailer adapter.
	Log Adapter = iota
	// SMTP is the SMTP mailer adapter.
	SMTP
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Log:
		return "log"
	case SMTP:
		return "smtp"
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
	case "smtp":
		return SMTP, nil
	default:
		return Log, &InvalidAdapterError{Adapter: s}
	}
}
