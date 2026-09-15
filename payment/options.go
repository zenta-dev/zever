package payment

import (
	"errors"
	"net/url"
)

const (
	// DefaultMaxWebhookBytes is the default byte limit for webhook payloads.
	DefaultMaxWebhookBytes = 1 << 20
	// DefaultHTTPTimeout is the default HTTP timeout in seconds for provider calls.
	DefaultHTTPTimeout = 30
)

// Options configures payment backend selection and limits.
type Options struct {
	// SecretKey holds the provider secret key. It is never logged.
	SecretKey string
	// APIKey holds the provider API key. It is never logged.
	APIKey string
	// WebhookSecret holds the webhook verification secret. It is never logged.
	WebhookSecret string
	// Endpoint holds the optional custom provider endpoint URL.
	Endpoint string
	// Sandbox selects the provider sandbox environment.
	Sandbox bool
	// AutoApprove marks new payments as succeeded instead of pending.
	AutoApprove bool
	// MaxWebhookBytes bounds the webhook payload size.
	MaxWebhookBytes int
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxWebhookBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max webhook bytes must be >= 0"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid URL"})
		} else {
			if u.Scheme == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include scheme"})
			}

			if u.Host == "" {
				errs = append(errs, &InvalidOptionsError{Reason: "endpoint must include host"})
			}
		}
	}

	return errors.Join(errs...)
}

// maxWebhookBytes returns MaxWebhookBytes or DefaultMaxWebhookBytes when unset.
func (o Options) maxWebhookBytes() int {
	if o.MaxWebhookBytes <= 0 {
		return DefaultMaxWebhookBytes
	}

	return o.MaxWebhookBytes
}
