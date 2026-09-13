package job

import "time"

// RetryPolicy controls how many attempts a job gets and the base delay between them.
type RetryPolicy struct {
	// MaxAttempts caps the total number of execution attempts for a job.
	MaxAttempts int
	// BaseDelay sets the initial retry delay doubled by Backoff on each attempt.
	BaseDelay time.Duration
}

// DefaultRetryPolicy returns the standard RetryPolicy of 25 attempts with a 5s base delay.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 25,
		BaseDelay:   5 * time.Second,
	}
}

// Backoff returns the exponential delay for the given attempt, capped at 24h. It caps the shift at 20 and returns BaseDelay for attempts below 1.
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if attempt < 1 {
		return p.BaseDelay
	}

	const maxDelay = 24 * time.Hour

	shift := uint(min(attempt-1, 20))
	if p.BaseDelay > maxDelay>>shift {
		return maxDelay
	}

	return p.BaseDelay * time.Duration(1<<shift)
}
