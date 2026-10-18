package notification

import (
	"time"
)

// DefaultTimeout is the default notification operation timeout applied by adapters.
const DefaultTimeout = 30 * time.Second

// TwilioOptions configures the Twilio SMS adapter.
type TwilioOptions struct {
	// AccountSID is the Twilio account SID for basic-auth and URL path.
	AccountSID string `json:"account_sid"`
	// AuthToken is the Twilio auth token for basic-auth.
	AuthToken string `json:"auth_token" zever:"secret"`
	// FromNumber is the Twilio sender phone number in E.164 format.
	FromNumber string `json:"from_number"`
}

// FCMOptions configures the Firebase Cloud Messaging push adapter.
type FCMOptions struct {
	// ProjectID is the Firebase project ID. Required.
	ProjectID string `json:"project_id"`
	// ServiceAccount is the path to the service account JSON key file. Required.
	ServiceAccount string `json:"service_account"`
}

// Options configures notifier construction.
type Options struct {
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration `json:"timeout"`
	// Twilio carries the Twilio SMS adapter settings.
	Twilio TwilioOptions `json:"twilio"`
	// FCM carries the Firebase Cloud Messaging push adapter settings.
	FCM FCMOptions `json:"fcm"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks options for consistency.
// Zero Timeout means "apply default" and is valid; only negative values fail.
func (o Options) Validate() error {
	if o.Timeout < 0 {
		return InvalidOptionsError{Reason: "timeout must be >= 0"}
	}
	return nil
}

// timeout returns the effective timeout, defaulting to DefaultTimeout.
func (o Options) timeout() time.Duration {
	if o.Timeout == 0 {
		return DefaultTimeout
	}
	return o.Timeout
}
