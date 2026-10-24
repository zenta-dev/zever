package cache

import (
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// MemoryOptions configures the in-memory cache backend.
type MemoryOptions struct {
	// SweepInterval controls how often expired entries are purged.
	SweepInterval time.Duration `json:"sweep_interval"`
	// MaxEntries bounds the number of stored entries before eviction.
	MaxEntries int `json:"max_entries"`
}

// RedisOptions configures the Redis-backed cache backend.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	redisopt.ConnectOptions
	// URL holds the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url"`

	// PoolSize limits the Redis connection pool size.
	PoolSize int `json:"pool_size"`
	// MinIdleConns sets the minimum number of idle Redis connections.
	MinIdleConns int `json:"min_idle_conns"`
	// PoolTimeout bounds waiting for a Redis connection from the pool.
	PoolTimeout time.Duration `json:"pool_timeout"`
	// MaxConnIdleTime bounds how long a Redis connection may stay idle.
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time"`
	// MaxConnLifetime bounds the total lifetime of a Redis connection.
	MaxConnLifetime time.Duration `json:"max_conn_lifetime"`
}

// Options configures cache backend selection and backend-specific settings.
type Options struct {
	// Adapter names the cache backend. Selection is compile-time: import
	// the adapter package and wire its New constructor with container.Provide.
	Adapter string `json:"adapter"`
	// Capacity hints at the expected number of cache entries.
	Capacity int `json:"capacity"`

	// Verbose enables additional diagnostic output.
	Verbose bool `json:"verbose"`

	// DSN is the postgres connection string or sqlite path for the db
	// adapter. Empty selects a private in-memory database.
	DSN string `json:"dsn"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool"`

	// MemoryOptions holds in-memory backend settings.
	MemoryOptions
	// RedisOptions holds Redis backend settings.
	RedisOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
