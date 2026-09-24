package payment

import (
	"errors"
	"net/url"
	"time"
)

const (
	// DefaultMaxWebhookBytes is the default byte limit for webhook payloads.
	DefaultMaxWebhookBytes = 1 << 20
	// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
	DefaultHTTPTimeout = 30 * time.Second
)

// Options configures payment backend selection and limits.
type Options struct {
	// SecretKey holds the provider secret key. It is never logged.
	SecretKey string `json:"secret_key" toml:"secret_key" yaml:"secret_key"`
	// APIKey holds the provider API key. It is never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// WebhookSecret holds the webhook verification secret. It is never logged.
	WebhookSecret string `json:"webhook_secret" toml:"webhook_secret" yaml:"webhook_secret"`
	// Endpoint holds the optional custom provider endpoint URL.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// Sandbox selects the provider sandbox environment.
	Sandbox bool `json:"sandbox" toml:"sandbox" yaml:"sandbox"`
	// AutoApprove marks new payments as succeeded instead of pending.
	AutoApprove bool `json:"auto_approve" toml:"auto_approve" yaml:"auto_approve"`
	// MaxWebhookBytes bounds the webhook payload size.
	MaxWebhookBytes int `json:"max_webhook_bytes" toml:"max_webhook_bytes" yaml:"max_webhook_bytes"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxWebhookBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_webhook_bytes must be >= 0"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "endpoint must be a valid url"})
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
