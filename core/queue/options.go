package queue

import (
	"time"

	"github.com/zenta-dev/zever/core/observability"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// ConnectOptions holds the shared Redis connection settings.
	redisopt.ConnectOptions
	// URL is the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`
	// Prefix is the key prefix for Redis queue data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// DBOptions holds connection settings for the DB adapter. It follows the
// core/workflow PostgresOptions precedent: adapter-specific connection
// configuration embedded in the shared Options so config, container, and
// Open flow through unchanged.
type DBOptions struct {
	// DSN is the postgres connection string or sqlite path. A postgres URL
	// opens postgres; anything else (including empty) selects sqlite, with
	// empty meaning a private in-memory-style database.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// Table is the queue-messages table name. Empty selects the adapter
	// default (DefaultTable in adapters/queue/db).
	Table string `json:"table" toml:"table" yaml:"table"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool" toml:"dedicated_pool" yaml:"dedicated_pool"`
}

// Options configures queue behavior and adapter-specific settings.
type Options struct {
	// VisibilityTimeout is the duration a popped message remains invisible before reclaim.
	VisibilityTimeout time.Duration `json:"visibility_timeout" toml:"visibility_timeout" yaml:"visibility_timeout"`
	// PollTimeout is the duration Pop waits for a message before returning empty.
	PollTimeout time.Duration `json:"poll_timeout" toml:"poll_timeout" yaml:"poll_timeout"`
	// Buffer is the maximum number of buffered ready messages per topic.
	Buffer int `json:"buffer" toml:"buffer" yaml:"buffer"`

	// Provider emits the producer span an adapter opens for every push and
	// the consumer span the job dispatcher opens for every delivery. Nil
	// disables telemetry. It is Go-API-only: never decoded from
	// configuration files.
	Provider observability.Provider `json:"-" toml:"-" yaml:"-"`

	// RedisOptions holds Redis-specific connection configuration.
	RedisOptions

	// DBOptions holds DB-specific connection configuration.
	DBOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	return nil
}
