package webhook

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/queue"
)

// Options configures webhook construction.
//
// Endpoint secrets are supplied per target at Register time, never in Options,
// and adapters must never log them.
//
// There is intentionally no APIKey field: endpoint authentication uses the
// per-target secret passed to Register, which scopes each credential to a
// single event target instead of sharing one key across all deliveries.
type Options struct {
	// Timeout is the per-delivery operation timeout. Zero means the adapter default.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// MaxRetries is the number of delivery retries after the initial attempt.
	// Zero means no retries.
	MaxRetries int `json:"max_retries" toml:"max_retries" yaml:"max_retries"`
	// AllowPrivateTargets permits http(s) targets resolving to private addresses.
	// It exists for tests and controlled networks; leave false in production.
	AllowPrivateTargets bool `json:"allow_private_targets" toml:"allow_private_targets" yaml:"allow_private_targets"`
	// QueueAdapter names the queue backend backing the queue adapter.
	QueueAdapter string `json:"queue_adapter" toml:"queue_adapter" yaml:"queue_adapter"`
	// QueueOpts carries the queue backend settings for the queue adapter.
	QueueOpts queue.Options `json:"queue_opts" toml:"queue_opts" yaml:"queue_opts"`
	// DeadLetterTopic is the queue topic for deliveries that exhaust retries.
	DeadLetterTopic string `json:"dead_letter_topic" toml:"dead_letter_topic" yaml:"dead_letter_topic"`
	// DSN is the SQLite database path for the sqlite adapter.
	// Empty means a unique private in-memory-style database.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// Logger emits background delivery warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
	// ReplayTolerance bounds how far a signature's embedded timestamp may
	// drift from the verifier's clock before verification rejects it as
	// expired or replayed, on top of the HMAC check itself. Zero means the
	// adapter default (5 minutes).
	ReplayTolerance time.Duration `json:"replay_tolerance" toml:"replay_tolerance" yaml:"replay_tolerance"`
}

// Validate checks options for consistency, joining all violations.
// Zero Timeout and zero MaxRetries are valid; only negative values fail.
//
// Validate checks options for consistency, joining all violations.
//
// Queue-specific rules (QueueAdapter required, QueueOpts visibility exceeding
// Timeout) are enforced by the queue adapter, not here: core stays
// backend-agnostic and the queue package owns its own invariants.
func (o Options) Validate() error {
	var errs []error
	if o.Timeout < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "timeout must be >= 0"})
	}
	if o.MaxRetries < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "max retries must be >= 0"})
	}
	if o.ReplayTolerance < 0 {
		errs = append(errs, &InvalidOptionsError{Reason: "replay tolerance must be >= 0"})
	}
	return errors.Join(errs...)
}
