package ratelimit

import (
	"math"
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

const (
	// DefaultIdleTTL is the default idle entry TTL applied by adapters.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval is the default idle-entry sweep interval applied by adapters for reaping idle entries.
	DefaultSweepInterval = time.Minute
	// MaxKeyLen is the maximum allowed rate-limit key length.
	MaxKeyLen = 256
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// Options holds the shared Redis connection AND pooling settings
	// (PoolSize, MinIdleConns, PoolTimeout, MaxConnIdleTime,
	// MaxConnLifetime) -- previously only ConnectOptions was embedded here,
	// silently dropping every pooling knob to go-redis's defaults with no
	// way for a caller to tune them for a hot ratelimit store.
	zredis.Options
	// Prefix is the key prefix for Redis ratelimit data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Options configures ratelimit behavior and adapter-specific settings.
type Options struct {
	// Rate is the token refill rate per second. Required, must be > 0 and finite.
	Rate float64 `json:"rate" toml:"rate" yaml:"rate"`
	// Burst is the maximum bucket size. Required, must be > 0.
	Burst int `json:"burst" toml:"burst" yaml:"burst"`
	// IdleTTL is the idle entry TTL. Zero means the adapter default; negative fails.
	IdleTTL time.Duration `json:"idle_ttl" toml:"idle_ttl" yaml:"idle_ttl"`
	// SweepInterval is the idle-entry sweep interval. Zero means the adapter default; negative fails.
	SweepInterval time.Duration `json:"sweep_interval" toml:"sweep_interval" yaml:"sweep_interval"`
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions `json:"redis" toml:"redis" yaml:"redis"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if math.IsNaN(o.Rate) || math.IsInf(o.Rate, 0) || o.Rate <= 0 {
		return &InvalidOptionsError{Reason: "rate must be > 0 and finite"}
	}

	if o.Burst <= 0 {
		return &InvalidOptionsError{Reason: "burst must be > 0"}
	}

	if o.IdleTTL < 0 {
		return &InvalidOptionsError{Reason: "idle_ttl must be >= 0"}
	}

	if o.SweepInterval < 0 {
		return &InvalidOptionsError{Reason: "sweep_interval must be >= 0"}
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
