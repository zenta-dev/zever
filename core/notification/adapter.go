package notification

// Adapter identifies the notification backend implementation.

type Adapter string

const (
	// Log is the log notification adapter.
	Log Adapter = "log"
	// Twilio is the Twilio SMS notification adapter.
	Twilio Adapter = "twilio"
	// FCM is the Firebase Cloud Messaging push adapter.
	FCM Adapter = "fcm"
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
