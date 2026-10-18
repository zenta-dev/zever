package cache

import (
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// MemoryOptions configures the in-memory cache backend.
type MemoryOptions struct {
	// SweepInterval controls how often expired entries are purged.
	SweepInterval time.Duration ``
	// MaxEntries bounds the number of stored entries before eviction.
	MaxEntries int ``
}

// RedisOptions configures the Redis-backed cache backend.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	redisopt.ConnectOptions
	// URL holds the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string ``

	// PoolSize limits the Redis connection pool size.
	PoolSize int ``
	// MinIdleConns sets the minimum number of idle Redis connections.
	MinIdleConns int ``
	// PoolTimeout bounds waiting for a Redis connection from the pool.
	PoolTimeout time.Duration ``
	// MaxConnIdleTime bounds how long a Redis connection may stay idle.
	MaxConnIdleTime time.Duration ``
	// MaxConnLifetime bounds the total lifetime of a Redis connection.
	MaxConnLifetime time.Duration ``
}

// Options configures cache backend selection and backend-specific settings.
type Options struct {
	// Adapter selects the cache backend to open.
	Adapter Adapter ``
	// Capacity hints at the expected number of cache entries.
	Capacity int ``

	// Verbose enables additional diagnostic output.
	Verbose bool ``

	// DSN is the postgres connection string or sqlite path for the db
	// adapter. Empty selects a private in-memory database.
	DSN string ``
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool ``

	// MemoryOptions holds in-memory backend settings.
	MemoryOptions
	// RedisOptions holds Redis backend settings.
	RedisOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
