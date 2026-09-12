package cache

import "time"

// MemoryOptions configures the in-memory cache backend.
type MemoryOptions struct {
	// SweepInterval controls how often expired entries are purged.
	SweepInterval time.Duration
	// MaxEntries bounds the number of stored entries before eviction.
	MaxEntries int
}

// RedisOptions configures the Redis-backed cache backend.
type RedisOptions struct {
	// URL holds the Redis connection URL.
	URL string
	// Addr holds the Redis server address.
	Addr string
	// Password holds the Redis authentication password.
	Password string
	// DB selects the Redis logical database.
	DB int

	// PoolSize limits the Redis connection pool size.
	PoolSize int
	// MinIdleConns sets the minimum number of idle Redis connections.
	MinIdleConns int
	// PoolTimeout bounds waiting for a Redis connection from the pool.
	PoolTimeout time.Duration
	// ConnMaxIdleTime bounds how long a Redis connection may stay idle.
	ConnMaxIdleTime time.Duration
	// ConnMaxLifetime bounds the total lifetime of a Redis connection.
	ConnMaxLifetime time.Duration
}

// Options configures cache backend selection and backend-specific settings.
type Options struct {
	// Adapter selects the cache backend to open.
	Adapter Adapter
	// Capacity hints at the expected number of cache entries.
	Capacity int

	// Verbose enables additional diagnostic output.
	Verbose bool

	// MemoryOptions holds in-memory backend settings.
	MemoryOptions
	// RedisOptions holds Redis backend settings.
	RedisOptions
}
