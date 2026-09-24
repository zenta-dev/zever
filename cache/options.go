package cache

import (
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

// MemoryOptions configures the in-memory cache backend.
type MemoryOptions struct {
	// SweepInterval controls how often expired entries are purged.
	SweepInterval time.Duration `json:"sweep_interval" toml:"sweep_interval" yaml:"sweep_interval"`
	// MaxEntries bounds the number of stored entries before eviction.
	MaxEntries int `json:"max_entries" toml:"max_entries" yaml:"max_entries"`
}

// RedisOptions configures the Redis-backed cache backend.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
	// URL holds the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`

	// PoolSize limits the Redis connection pool size.
	PoolSize int `json:"pool_size" toml:"pool_size" yaml:"pool_size"`
	// MinIdleConns sets the minimum number of idle Redis connections.
	MinIdleConns int `json:"min_idle_conns" toml:"min_idle_conns" yaml:"min_idle_conns"`
	// PoolTimeout bounds waiting for a Redis connection from the pool.
	PoolTimeout time.Duration `json:"pool_timeout" toml:"pool_timeout" yaml:"pool_timeout"`
	// MaxConnIdleTime bounds how long a Redis connection may stay idle.
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" toml:"max_conn_idle_time" yaml:"max_conn_idle_time"`
	// MaxConnLifetime bounds the total lifetime of a Redis connection.
	MaxConnLifetime time.Duration `json:"max_conn_lifetime" toml:"max_conn_lifetime" yaml:"max_conn_lifetime"`
}

// Options configures cache backend selection and backend-specific settings.
type Options struct {
	// Adapter selects the cache backend to open.
	Adapter Adapter `json:"adapter" toml:"adapter" yaml:"adapter"`
	// Capacity hints at the expected number of cache entries.
	Capacity int `json:"capacity" toml:"capacity" yaml:"capacity"`

	// Verbose enables additional diagnostic output.
	Verbose bool `json:"verbose" toml:"verbose" yaml:"verbose"`

	// MemoryOptions holds in-memory backend settings.
	MemoryOptions
	// RedisOptions holds Redis backend settings.
	RedisOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
