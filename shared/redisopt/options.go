package redisopt

import (
	"time"
)

// Options configures the Redis connection and pooling behavior.
type Options struct {
	// URL is reserved and currently ignored.
	URL string `json:"url" toml:"url" yaml:"url"`
	// ConnectOptions holds the shared connection settings.
	ConnectOptions

	// PoolSize is the base number of socket connections. <= 0 uses go-redis's
	// own default (10 * GOMAXPROCS).
	PoolSize int `json:"pool_size" toml:"pool_size" yaml:"pool_size"`
	// MinIdleConns is the minimum number of idle connections to keep
	// pre-warmed. <= 0 defaults to a small warm pool of 2, to avoid
	// cold-start dial latency on the first requests.
	MinIdleConns int `json:"min_idle_conns" toml:"min_idle_conns" yaml:"min_idle_conns"`
	// PoolTimeout is how long a caller waits for a connection when the pool
	// is exhausted. <= 0 uses go-redis's own default.
	PoolTimeout time.Duration `json:"pool_timeout" toml:"pool_timeout" yaml:"pool_timeout"`
	// MaxConnIdleTime is the maximum time a connection may sit idle before
	// it's eligible for closing. <= 0 uses go-redis's own default.
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" toml:"max_conn_idle_time" yaml:"max_conn_idle_time"`
	// MaxConnLifetime is the maximum time a connection may be reused before
	// it's eligible for closing. <= 0 uses go-redis's own default (unbounded).
	MaxConnLifetime time.Duration `json:"max_conn_lifetime" toml:"max_conn_lifetime" yaml:"max_conn_lifetime"`
}
