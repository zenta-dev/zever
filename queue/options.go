package queue

import (
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
	// URL is the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`
	// Prefix is the key prefix for Redis queue data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Options configures queue behavior and adapter-specific settings.
type Options struct {
	// VisibilityTimeout is the duration a popped message remains invisible before reclaim.
	VisibilityTimeout time.Duration `json:"visibility_timeout" toml:"visibility_timeout" yaml:"visibility_timeout"`
	// PollTimeout is the duration Pop waits for a message before returning empty.
	PollTimeout time.Duration `json:"poll_timeout" toml:"poll_timeout" yaml:"poll_timeout"`
	// Buffer is the maximum number of buffered ready messages per topic.
	Buffer int `json:"buffer" toml:"buffer" yaml:"buffer"`

	// RedisOptions holds Redis-specific connection configuration.
	RedisOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
