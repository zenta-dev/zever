package notification

// Adapter identifies the notification backend implementation.
type Adapter int

const (
	// Log is the log notification adapter.
	Log Adapter = iota
	// Twilio is the Twilio SMS notification adapter.
	Twilio
	// FCM is the Firebase Cloud Messaging push adapter.
	FCM
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Log:
		return "log"
	case Twilio:
		return "twilio"
	case FCM:
		return "fcm"
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
	case "twilio":
		return Twilio, nil
	case "fcm":
		return FCM, nil
	default:
		return Log, &InvalidAdapterError{Adapter: s}
	}
}
