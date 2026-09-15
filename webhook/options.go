package webhook

import (
	"errors"
	"time"

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
	Timeout time.Duration
	// MaxRetries is the number of delivery retries after the initial attempt.
	// Zero means no retries.
	MaxRetries int
	// AllowPrivateTargets permits http(s) targets resolving to private addresses.
	// It exists for tests and controlled networks; leave false in production.
	AllowPrivateTargets bool
	// QueueAdapter names the queue backend backing the queue adapter.
	QueueAdapter string
	// QueueOpts carries the queue backend settings for the queue adapter.
	QueueOpts queue.Options
	// DeadLetterTopic is the queue topic for deliveries that exhaust retries.
	DeadLetterTopic string
	// DSN is the SQLite database path for the sqlite adapter.
	// Empty means a unique private in-memory-style database.
	DSN string
}

// Validate checks options for consistency.
// Zero Timeout and zero MaxRetries are valid; only negative values fail.
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
	return errors.Join(errs...)
}
