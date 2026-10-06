package resilience

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/shared/redisopt"
	"github.com/zenta-dev/zever/shared/retry"
)

const (
	// DefaultTimeout is the default overall operation timeout.
	DefaultTimeout = 5 * time.Second
	// DefaultRetryBaseDelay is the default delay before the first retry.
	DefaultRetryBaseDelay = 50 * time.Millisecond
	// DefaultRetryMaxDelay caps the computed retry backoff.
	DefaultRetryMaxDelay = 2 * time.Second
	// DefaultRetryMultiplier scales the retry backoff per attempt.
	DefaultRetryMultiplier = 2
	// DefaultRetryJitter is the default fraction of retry delay randomized.
	DefaultRetryJitter = 0.1
	// DefaultRetryMaxAttempts is the default total number of attempts.
	DefaultRetryMaxAttempts = 3
	// DefaultBreakerMaxRequests is the default half-open probe allowance.
	DefaultBreakerMaxRequests = 1
	// DefaultBreakerInterval is the default closed-state counting window.
	DefaultBreakerInterval = 30 * time.Second
	// DefaultBreakerBucketPeriod is the default rolling-window bucket width.
	DefaultBreakerBucketPeriod = 10 * time.Second
	// DefaultBreakerTimeout is the default open-state duration before a probe.
	DefaultBreakerTimeout = 30 * time.Second
	// DefaultBreakerMinRequests is the default minimum requests before a ratio trip.
	DefaultBreakerMinRequests = 10
	// DefaultBreakerFailureRatio is the default failure ratio that trips the breaker.
	DefaultBreakerFailureRatio = 0.5
	// DefaultBreakerConsecutiveFailures is the default consecutive-failure trip threshold.
	DefaultBreakerConsecutiveFailures = 5
	// DefaultMaxConcurrent is the default number of concurrent bulkhead slots.
	DefaultMaxConcurrent = 100
	// DefaultMaxQueue is the default number of bulkhead waiters.
	DefaultMaxQueue = 100
	// DefaultMaxWait is the default bulkhead admission wait budget.
	DefaultMaxWait = time.Second
	// DefaultRedisPrefix is the default Redis key namespace for the redis adapter.
	DefaultRedisPrefix = "resilience"
)

// RedisOptions holds connection settings for the Redis resilience adapter.
type RedisOptions struct {
	// Options holds the shared Redis connection and pooling settings.
	redisopt.Options
	// Prefix is the key namespace for Redis resilience data. Empty means
	// DefaultRedisPrefix.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// BreakerOptions configures the circuit breaker. Enabled false disables the
// breaker entirely. MinRequests, FailureRatio, and ConsecutiveFailures translate
// to a trip predicate: the breaker opens when either the consecutive-failure
// threshold is reached or the request count meets MinRequests and the failure
// ratio meets FailureRatio.
type BreakerOptions struct {
	// Enabled turns the circuit breaker on.
	Enabled bool `json:"enabled" toml:"enabled" yaml:"enabled"`
	// MaxRequests is the number of probes allowed while half-open.
	MaxRequests uint32 `json:"max_requests" toml:"max_requests" yaml:"max_requests"`
	// Interval is the closed-state window before counts reset.
	Interval time.Duration `json:"interval" toml:"interval" yaml:"interval"`
	// BucketPeriod is the rolling-window bucket width; Interval becomes a multiple of it.
	BucketPeriod time.Duration `json:"bucket_period" toml:"bucket_period" yaml:"bucket_period"`
	// Timeout is the open-state duration before the breaker half-opens.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// MinRequests is the minimum request count before the failure ratio applies.
	MinRequests int `json:"min_requests" toml:"min_requests" yaml:"min_requests"`
	// FailureRatio is the failure ratio in [0,1] that trips the breaker.
	FailureRatio float64 `json:"failure_ratio" toml:"failure_ratio" yaml:"failure_ratio"`
	// ConsecutiveFailures trips the breaker after this many consecutive failures.
	ConsecutiveFailures int `json:"consecutive_failures" toml:"consecutive_failures" yaml:"consecutive_failures"`
	// IsSuccessful classifies a non-nil error as success when it returns true.
	IsSuccessful func(error) bool `json:"-" toml:"-" yaml:"-"`
}

// BulkheadOptions configures the bulkhead. MaxConcurrent <= 0 disables it.
// MaxQueue <= 0 leaves the waiter count unbounded; MaxWait <= 0 waits until
// the caller context is done.
type BulkheadOptions struct {
	// MaxConcurrent is the number of concurrent executions admitted.
	MaxConcurrent int `json:"max_concurrent" toml:"max_concurrent" yaml:"max_concurrent"`
	// MaxQueue bounds the number of waiters; <= 0 is unbounded.
	MaxQueue int `json:"max_queue" toml:"max_queue" yaml:"max_queue"`
	// MaxWait bounds how long a caller waits for a slot; <= 0 waits indefinitely.
	MaxWait time.Duration `json:"max_wait" toml:"max_wait" yaml:"max_wait"`
}

// Options configures resilience behavior for a Manager and the Guards it creates.
type Options struct {
	// Name is the base name applied to the breaker.
	Name string `json:"name" toml:"name" yaml:"name"`
	// Timeout bounds the whole operation, including retries.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// Retry configures retry backoff; MaxAttempts <= 1 disables retries.
	Retry retry.Policy `json:"retry" toml:"retry" yaml:"retry"`
	// Breaker configures the circuit breaker.
	Breaker BreakerOptions `json:"breaker" toml:"breaker" yaml:"breaker"`
	// Bulkhead configures the bulkhead.
	Bulkhead BulkheadOptions `json:"bulkhead" toml:"bulkhead" yaml:"bulkhead"`
	// OnStateChange is called on every circuit-breaker state transition.
	OnStateChange func(name string, from, to State) `json:"-" toml:"-" yaml:"-"`
	// Redis holds Redis-specific connection configuration for the redis adapter.
	Redis RedisOptions `json:"redis" toml:"redis" yaml:"redis"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if o.Retry.BaseDelay < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "retry base_delay must be >= 0"})
	}

	if o.Retry.MaxDelay < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "retry max_delay must be >= 0"})
	}

	if o.Retry.MaxAttempts < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "retry max_attempts must be >= 0"})
	}

	if o.Retry.Jitter < 0 || o.Retry.Jitter > 1 {
		errs = append(errs, InvalidOptionsError{Reason: "retry jitter must be in [0, 1]"})
	}

	if o.Breaker.Enabled {
		if o.Breaker.FailureRatio < 0 || o.Breaker.FailureRatio > 1 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker failure_ratio must be in [0, 1]"})
		}

		if o.Breaker.MinRequests < 0 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker min_requests must be >= 0"})
		}

		if o.Breaker.ConsecutiveFailures < 0 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker consecutive_failures must be >= 0"})
		}

		if o.Breaker.Interval < 0 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker interval must be >= 0"})
		}

		if o.Breaker.BucketPeriod < 0 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker bucket_period must be >= 0"})
		}

		if o.Breaker.Timeout < 0 {
			errs = append(errs, InvalidOptionsError{Reason: "breaker timeout must be >= 0"})
		}
	}

	if o.Bulkhead.MaxConcurrent < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "bulkhead max_concurrent must be >= 0"})
	}

	if o.Bulkhead.MaxQueue < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "bulkhead max_queue must be >= 0"})
	}

	if o.Bulkhead.MaxWait < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "bulkhead max_wait must be >= 0"})
	}

	if o.Redis.Prefix != "" {
		if err := redisopt.ValidatePrefix(o.Redis.Prefix); err != nil {
			errs = append(errs, InvalidOptionsError{Reason: err.Error()})
		}
	}

	return errors.Join(errs...)
}

// Default returns zero-infra options with every policy enabled and bounded.
func Default() Options {
	return Options{
		Timeout: DefaultTimeout,
		Retry: retry.Policy{
			BaseDelay:   DefaultRetryBaseDelay,
			MaxDelay:    DefaultRetryMaxDelay,
			Multiplier:  DefaultRetryMultiplier,
			Jitter:      DefaultRetryJitter,
			MaxAttempts: DefaultRetryMaxAttempts,
		},
		Breaker: BreakerOptions{
			Enabled:             true,
			MaxRequests:         DefaultBreakerMaxRequests,
			Interval:            DefaultBreakerInterval,
			BucketPeriod:        DefaultBreakerBucketPeriod,
			Timeout:             DefaultBreakerTimeout,
			MinRequests:         DefaultBreakerMinRequests,
			FailureRatio:        DefaultBreakerFailureRatio,
			ConsecutiveFailures: DefaultBreakerConsecutiveFailures,
		},
		Bulkhead: BulkheadOptions{
			MaxConcurrent: DefaultMaxConcurrent,
			MaxQueue:      DefaultMaxQueue,
			MaxWait:       DefaultMaxWait,
		},
	}
}
