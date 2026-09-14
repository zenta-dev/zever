package eventbus

import (
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBufferSize is the default per-topic buffer size applied by adapters.
	DefaultBufferSize = 1024
	// DefaultMaxHandlers is the default maximum handler count applied by adapters.
	DefaultMaxHandlers = 128
	// DefaultHandlerTimeout is the default per-handler delivery timeout applied by adapters.
	DefaultHandlerTimeout = 30 * time.Second
	// DefaultCloseTimeout is the default shutdown timeout applied by adapters.
	DefaultCloseTimeout = 5 * time.Second
	// MaxTopicLen is the maximum allowed topic length.
	MaxTopicLen = 256
	// MaxMessageSize is the maximum allowed message payload size in bytes.
	MaxMessageSize = 4 << 20
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
	// Prefix is the key prefix for Redis eventbus data.
	Prefix string
}

// Options configures eventbus behavior and adapter-specific settings.
type Options struct {
	// BufferSize is the per-topic buffer size. Zero means the adapter default.
	BufferSize int
	// MaxHandlers is the maximum handler count. Zero means the adapter default.
	MaxHandlers int
	// HandlerTimeout is the per-handler delivery timeout. Zero means the adapter default.
	HandlerTimeout time.Duration
	// CloseTimeout is the shutdown timeout. Zero means the adapter default.
	CloseTimeout time.Duration
	// OnPanic handles handler panics. It is never validated.
	OnPanic func(topic string, msg Message, r any)
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions
}

// Validate checks options for consistency.
// Zero BufferSize, MaxHandlers, HandlerTimeout, and CloseTimeout mean
// "apply adapter defaults" and are valid; only negative values fail.
func (o Options) Validate() error {
	if o.BufferSize < 0 {
		return &InvalidOptionsError{Reason: "buffer size must be >= 0"}
	}

	if o.MaxHandlers < 0 {
		return &InvalidOptionsError{Reason: "max handlers must be >= 0"}
	}

	if o.HandlerTimeout < 0 {
		return &InvalidOptionsError{Reason: "handler timeout must be >= 0"}
	}

	if o.CloseTimeout < 0 {
		return &InvalidOptionsError{Reason: "close timeout must be >= 0"}
	}

	if err := validateRedisAddr(o.Redis.Addr); err != nil {
		return err
	}

	if err := validateRedisPrefix(o.Redis.Prefix); err != nil {
		return err
	}

	return nil
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
