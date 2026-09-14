package idempotency

import (
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTTL is the default reservation lifetime applied by adapters.
	DefaultTTL = 24 * time.Hour
	// MaxKeyLen is the maximum allowed idempotency key length in bytes.
	MaxKeyLen = 255
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
	// Prefix is the key prefix for Redis idempotency data.
	Prefix string
}

// Options configures idempotency store construction.
type Options struct {
	// TTL is the default reservation lifetime. Zero means DefaultTTL.
	TTL time.Duration
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions
}

// Validate checks options for consistency.
// Zero TTL means DefaultTTL and is valid; only negative TTL fails.
func (o Options) Validate() error {
	if o.TTL < 0 {
		return &InvalidOptionsError{Reason: "ttl must be >= 0"}
	}

	if err := validateRedisAddr(o.Redis.Addr); err != nil {
		return err
	}

	if err := validateRedisPrefix(o.Redis.Prefix); err != nil {
		return err
	}

	return nil
}

// ttl returns the effective reservation lifetime.
// Zero means DefaultTTL. Negative values fail Validate and are
// never resolved here; callers must Validate first.
func (o Options) ttl() time.Duration {
	if o.TTL == 0 {
		return DefaultTTL
	}

	return o.TTL
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
