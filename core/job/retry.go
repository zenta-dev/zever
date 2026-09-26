package job

import (
	"time"

	"github.com/zenta-dev/zever/internal/retry"
)

// RetryPolicy controls how many attempts a job gets and the base delay between them.
type RetryPolicy struct {
	// MaxAttempts caps the total number of execution attempts for a job.
	MaxAttempts int
	// BaseDelay sets the initial retry delay doubled by Backoff on each attempt.
	BaseDelay time.Duration
}

// DefaultBaseDelay is the initial retry delay doubled by Backoff.
const DefaultBaseDelay = 5 * time.Second

// MaxBackoffDelay caps the exponential retry delay.
const MaxBackoffDelay = 24 * time.Hour

// DefaultRetryPolicy returns the standard RetryPolicy of 25 attempts with a 5s base delay.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 25,
		BaseDelay:   DefaultBaseDelay,
	}
}

// maxBackoffShiftAttempt is the attempt at which Backoff's growth freezes:
// the original hand-rolled implementation capped its doubling exponent
// (shift) at 20 rather than capping the resulting duration itself, so an
// attempt beyond this point keeps producing the same delay as attempt 21
// even when that delay is still well under the 24h ceiling. Clamping the
// attempt passed to retry.Policy.NextDelay here reproduces that exact
// quirk instead of only capping at 24h (which would change results for a
// small BaseDelay at a high attempt count).
const maxBackoffShiftAttempt = 21

// Backoff returns the exponential delay for the given attempt, capped at 24h. It caps the shift at 20 and returns BaseDelay for attempts below 1.
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	if attempt > maxBackoffShiftAttempt {
		attempt = maxBackoffShiftAttempt
	}

	return retry.Policy{BaseDelay: p.BaseDelay, Multiplier: 2, MaxDelay: MaxBackoffDelay}.NextDelay(attempt)
}
