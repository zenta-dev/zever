package ratelimit

import (
	"math"
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

const (
	// DefaultIdleTTL is the default idle entry TTL applied by adapters.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval is the default idle-entry sweep interval applied by adapters for reaping idle entries.
	DefaultSweepInterval = time.Minute
	// DefaultMaxEntries bounds the memory adapter's bucket table before LRU eviction.
	DefaultMaxEntries = 1000
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
	redisopt.Options
	// Prefix is the key prefix for Redis ratelimit data.
	Prefix string `json:"prefix"`
}

// Options configures ratelimit behavior and adapter-specific settings.
type Options struct {
	// Rate is the token refill rate per second. Required, must be > 0 and finite.
	Rate float64 `json:"rate"`
	// Burst is the maximum bucket size. Required, must be > 0.
	Burst int `json:"burst"`
	// IdleTTL is the idle entry TTL. Zero means the adapter default; negative fails.
	IdleTTL time.Duration `json:"idle_ttl"`
	// SweepInterval is the idle-entry sweep interval. Zero means the adapter default; negative fails.
	SweepInterval time.Duration `json:"sweep_interval"`
	// MaxEntries bounds the number of stored buckets in the memory adapter
	// before least-recently-used eviction. Non-positive means DefaultMaxEntries.
	MaxEntries int `json:"max_entries"`
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions `json:"redis"`
	// TrustedProxies lists CIDRs (or bare IPs) of proxies whose
	// X-Forwarded-For headers are honored for rate-limit keys. Empty
	// means no proxy is trusted and keys always derive from the peer
	// address. See ParseTrustedProxies.
	TrustedProxies []string `json:"trusted_proxies"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if math.IsNaN(o.Rate) || math.IsInf(o.Rate, 0) || o.Rate <= 0 {
		return InvalidOptionsError{Reason: "rate must be > 0 and finite"}
	}

	if o.Burst <= 0 {
		return InvalidOptionsError{Reason: "burst must be > 0"}
	}

	if o.IdleTTL < 0 {
		return InvalidOptionsError{Reason: "idle_ttl must be >= 0"}
	}

	if o.SweepInterval < 0 {
		return InvalidOptionsError{Reason: "sweep_interval must be >= 0"}
	}

	if err := redisopt.ValidateAddr(o.Redis.Addr); err != nil {
		return InvalidOptionsError{Reason: err.Error()}
	}

	if err := redisopt.ValidatePrefix(o.Redis.Prefix); err != nil {
		return InvalidOptionsError{Reason: err.Error()}
	}

	if _, err := ParseTrustedProxies(o.TrustedProxies); err != nil {
		return err
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
