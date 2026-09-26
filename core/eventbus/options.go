package eventbus

import (
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
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
	// Options holds the shared Redis connection AND pooling settings
	// (PoolSize, MinIdleConns, PoolTimeout, MaxConnIdleTime,
	// MaxConnLifetime) -- previously only ConnectOptions was embedded here,
	// silently dropping every pooling knob to go-redis's defaults with no
	// way for a caller to tune them for a hot eventbus.
	redisopt.Options
	// Prefix is the key prefix for Redis eventbus data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Options configures eventbus behavior and adapter-specific settings.
type Options struct {
	// BufferSize is the per-topic buffer size. Zero means the adapter default.
	BufferSize int `json:"buffer_size" toml:"buffer_size" yaml:"buffer_size"`
	// MaxHandlers is the maximum handler count. Zero means the adapter default.
	MaxHandlers int `json:"max_handlers" toml:"max_handlers" yaml:"max_handlers"`
	// HandlerTimeout is the per-handler delivery timeout. Zero means the adapter default.
	HandlerTimeout time.Duration `json:"handler_timeout" toml:"handler_timeout" yaml:"handler_timeout"`
	// CloseTimeout is the shutdown timeout. Zero means the adapter default.
	CloseTimeout time.Duration `json:"close_timeout" toml:"close_timeout" yaml:"close_timeout"`
	// OnPanic handles handler panics. It is never validated.
	OnPanic func(topic string, msg Message, r any) `json:"-" toml:"-" yaml:"-"`
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions `json:"redis" toml:"redis" yaml:"redis"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks options for consistency.
// Zero BufferSize, MaxHandlers, HandlerTimeout, and CloseTimeout mean
// "apply adapter defaults" and are valid; only negative values fail.
func (o Options) Validate() error {
	if o.BufferSize < 0 {
		return &InvalidOptionsError{Reason: "buffer_size must be >= 0"}
	}

	if o.MaxHandlers < 0 {
		return &InvalidOptionsError{Reason: "max_handlers must be >= 0"}
	}

	if o.HandlerTimeout < 0 {
		return &InvalidOptionsError{Reason: "handler_timeout must be >= 0"}
	}

	if o.CloseTimeout < 0 {
		return &InvalidOptionsError{Reason: "close_timeout must be >= 0"}
	}

	if err := redisopt.ValidateAddr(o.Redis.Addr); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	if err := redisopt.ValidatePrefix(o.Redis.Prefix); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	return nil
}
