package payment

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/internal/providers"
)

const (
	// DefaultMaxWebhookBytes is the default byte limit for webhook payloads.
	DefaultMaxWebhookBytes = 1 << 20
	// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
	// Alias for providers.DefaultHTTPTimeout, which billing.DefaultHTTPTimeout
	// also aliases -- previously both packages independently declared their
	// own identical constant.
	DefaultHTTPTimeout = providers.DefaultHTTPTimeout
	// DefaultRefundIdempotencyTTL bounds refund idempotency reservations.
	DefaultRefundIdempotencyTTL = 24 * time.Hour
)

// Options configures payment backend selection and limits. SecretKey/
// APIKey/Endpoint/Sandbox are shared with billing.Options via
// providers.Common, validated identically in both packages.
type Options struct {
	providers.Common
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

	for _, e := range providers.ValidateEndpoint(o.Endpoint) {
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
