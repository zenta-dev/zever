package ratelimit

import (
	"math"
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

const (
	// DefaultIdleTTL is the default idle entry TTL applied by adapters.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval is the default idle-entry sweep interval applied by adapters.
	DefaultSweepInterval = time.Minute
	// MaxKeyLen is the maximum allowed rate-limit key length.
	MaxKeyLen = 256
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
	// Prefix is the key prefix for Redis ratelimit data.
	Prefix string
}

// Options configures ratelimit behavior and adapter-specific settings.
type Options struct {
	// Rate is the token refill rate per second. Required, must be > 0 and finite.
	Rate float64
	// Burst is the maximum bucket size. Required, must be > 0.
	Burst int
	// IdleTTL is the idle entry TTL. Zero means the adapter default; negative fails.
	IdleTTL time.Duration
	// SweepInterval is the idle-entry sweep interval. Zero means the adapter default; negative fails.
	SweepInterval time.Duration
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions
}

// Validate checks options for consistency.
func (o Options) Validate() error {
	if math.IsNaN(o.Rate) || math.IsInf(o.Rate, 0) || o.Rate <= 0 {
		return &InvalidOptionsError{Reason: "rate must be > 0 and finite"}
	}

	if o.Burst <= 0 {
		return &InvalidOptionsError{Reason: "burst must be > 0"}
	}

	if o.IdleTTL < 0 {
		return &InvalidOptionsError{Reason: "idle TTL must be >= 0"}
	}

	if o.SweepInterval < 0 {
		return &InvalidOptionsError{Reason: "sweep interval must be >= 0"}
	}

	if err := zredis.ValidateAddr(o.Redis.Addr); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	if err := zredis.ValidatePrefix(o.Redis.Prefix); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	return nil
}

// idleTTL returns IdleTTL or DefaultIdleTTL when unset.
func (o Options) idleTTL() time.Duration {
	if o.IdleTTL <= 0 {
		return DefaultIdleTTL
	}

	return o.IdleTTL
}

// sweepInterval returns SweepInterval or DefaultSweepInterval when unset.
func (o Options) sweepInterval() time.Duration {
	if o.SweepInterval <= 0 {
		return DefaultSweepInterval
	}

	return o.SweepInterval
}
