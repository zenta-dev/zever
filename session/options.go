package session

import (
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

const (
	// DefaultTTL is the default session lifetime applied by adapters.
	DefaultTTL = 15 * time.Minute
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
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

// Validate checks options for consistency.
// Zero TTL means DefaultTTL and is valid; only negative TTL fails.
func (o Options) Validate() error {
	if o.TTL < 0 {
		return &InvalidOptionsError{Reason: "ttl must be >= 0"}
	}
	if err := zredis.ValidateAddr(o.Redis.Addr); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}
	if err := zredis.ValidatePrefix(o.Redis.Prefix); err != nil {
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
