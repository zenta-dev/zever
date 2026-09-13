package queue

import "time"

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// URL is the Redis connection URL.
	URL string
	// Addr is the Redis server address.
	Addr string
	// Password is the Redis authentication password.
	Password string
	// DB is the Redis database index.
	DB int
	// Prefix is the key prefix for Redis queue data.
	Prefix string
}

// Options configures queue behavior and adapter-specific settings.
type Options struct {
	// VisibilityTimeout is the duration a popped message remains invisible before reclaim.
	VisibilityTimeout time.Duration
	// PollTimeout is the duration Pop waits for a message before returning empty.
	PollTimeout time.Duration
	// Buffer is the maximum number of buffered ready messages per topic.
	Buffer int

	// RedisOptions holds Redis-specific connection configuration.
	RedisOptions
}
