package session

import (
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

const (
	// DefaultTTL is the default session lifetime applied by adapters.
	DefaultTTL = 15 * time.Minute
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// Options holds the shared Redis connection AND pooling settings
	// (PoolSize, MinIdleConns, PoolTimeout, MaxConnIdleTime,
	// MaxConnLifetime) -- previously only ConnectOptions was embedded here,
	// silently dropping every pooling knob to go-redis's defaults with no
	// way for a caller to tune them for a hot session store.
	redisopt.Options
	// Prefix is the key prefix for Redis session data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Options configures session store construction.
type Options struct {
	// TTL is the default session lifetime. Zero means DefaultTTL.
	TTL time.Duration `json:"ttl" toml:"ttl" yaml:"ttl"`
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions `json:"redis" toml:"redis" yaml:"redis"`
}

// Validate checks options for consistency, joining all violations.
// Zero TTL means DefaultTTL and is valid; only negative TTL fails.
func (o Options) Validate() error {
	if o.TTL < 0 {
		return &InvalidOptionsError{Reason: "ttl must be >= 0"}
	}
	if err := redisopt.ValidateAddr(o.Redis.Addr); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}
	if err := redisopt.ValidatePrefix(o.Redis.Prefix); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	return nil
}

// ttl returns the effective session lifetime.
// Zero means DefaultTTL. Negative values fail Validate and are
// never resolved here; callers must Validate first.
func (o Options) ttl() time.Duration {
	if o.TTL == 0 {
		return DefaultTTL
	}

	return o.TTL
}
