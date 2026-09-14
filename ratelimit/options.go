package ratelimit

import (
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultIdleTTL is the default idle entry TTL applied by adapters.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval is the default idle-entry sweep interval applied by adapters.
	DefaultSweepInterval = time.Minute
	// MaxKeyLen is the maximum allowed rate-limit key length.
	MaxKeyLen = 256
	// maxRedisPrefixLen is the maximum allowed Redis key prefix length.
	maxRedisPrefixLen = 64
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// Addr is the Redis server address in host:port form.
	// Empty means the adapter default (localhost:6379).
	Addr string
	// Password is the Redis authentication password.
	Password string
	// DB is the Redis database index.
	DB int
	// TLS enables TLS for the Redis connection.
	TLS bool
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

	if err := validateRedisAddr(o.Redis.Addr); err != nil {
		return err
	}

	if err := validateRedisPrefix(o.Redis.Prefix); err != nil {
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

// validateRedisAddr checks a Redis host:port address.
// Empty means the adapter default and is valid.
func validateRedisAddr(addr string) error {
	if addr == "" {
		return nil
	}

	if strings.Contains(addr, "://") {
		return &InvalidOptionsError{Reason: "redis addr must be host:port without scheme"}
	}

	if strings.ContainsAny(addr, "/?#") {
		return &InvalidOptionsError{Reason: "redis addr must be host:port"}
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return &InvalidOptionsError{Reason: "redis addr must be host:port"}
	}

	if host == "" {
		return &InvalidOptionsError{Reason: "redis addr host must be non-empty"}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return &InvalidOptionsError{Reason: "redis addr port must be 1-65535"}
	}

	return nil
}

// validateRedisPrefix checks the Redis key prefix: at most 64 token characters.
func validateRedisPrefix(prefix string) error {
	if len(prefix) > maxRedisPrefixLen {
		return &InvalidOptionsError{Reason: "redis prefix must be at most 64 characters"}
	}

	for i := 0; i < len(prefix); i++ {
		if !isTokenChar(prefix[i]) {
			return &InvalidOptionsError{Reason: "redis prefix must contain only token characters"}
		}
	}

	return nil
}

// isTokenChar reports whether c is an RFC 7230 token character.
func isTokenChar(c byte) bool {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return true
	}

	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}

	return false
}
