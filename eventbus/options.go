package eventbus

import (
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
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
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
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

	if err := zredis.ValidateAddr(o.Redis.Addr); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	if err := zredis.ValidatePrefix(o.Redis.Prefix); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	return nil
}
