package payment

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/shared/providersopt"
)

const (
	// DefaultMaxWebhookBytes is the default byte limit for webhook payloads.
	DefaultMaxWebhookBytes = 1 << 20
	// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
	// Alias for providersopt.DefaultHTTPTimeout, which billing.DefaultHTTPTimeout
	// also aliases -- previously both packages independently declared their
	// own identical constant.
	DefaultHTTPTimeout = providersopt.DefaultHTTPTimeout
	// DefaultRefundIdempotencyTTL bounds refund idempotency reservations.
	DefaultRefundIdempotencyTTL = 24 * time.Hour
)

// Options configures payment backend selection and limits. SecretKey/
// APIKey/Endpoint/Sandbox are shared with billing.Options via
// providersopt.Common, validated identically in both packages.
type Options struct {
	providersopt.Common
	// WebhookSecret holds the webhook verification secret. It is never logged.
	WebhookSecret string `json:"webhook_secret" toml:"webhook_secret" yaml:"webhook_secret"`
	// AutoApprove marks new payments as succeeded instead of pending.
	AutoApprove bool `json:"auto_approve" toml:"auto_approve" yaml:"auto_approve"`
	// MaxWebhookBytes bounds the webhook payload size.
	MaxWebhookBytes int `json:"max_webhook_bytes" toml:"max_webhook_bytes" yaml:"max_webhook_bytes"`
	// Idempotency holds the optional refund idempotency store. Nil skips the guard.
	Idempotency idempotency.Store
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxWebhookBytes < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max_webhook_bytes must be >= 0"})
	}

	for _, e := range providersopt.ValidateEndpoint(o.Endpoint) {
		errs = append(errs, &InvalidOptionsError{Reason: e.Error()})
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
