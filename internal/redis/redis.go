package redis

import (
	"fmt"
	"sync"

	goredis "github.com/redis/go-redis/v9"
)

// Pool is a singleton holder for the shared Redis client and its options.
type Pool struct {
	client *goredis.Client
	opt    Options

	closed bool
}

var (
	instance *Pool
	mu       sync.RWMutex
)

// New returns the shared Redis client for the given options. It reuses the existing client when options match and closes the old client before replacing it. It returns the client or a wrapped error.
func New(opts Options) (*goredis.Client, error) {
	mu.Lock()
	defer mu.Unlock()

	// Reuse an existing client when configuration matches.
	if instance != nil && !instance.closed && opts.Compare(instance.opt) {
		return instance.client, nil
	}

	// Close the old client before replacing it.
	if instance != nil && instance.client != nil {
		if err := instance.client.Close(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrCloseClient, err)
		}
	}

	redisOpt, err := opts.toRedisOptions()
	if err != nil {
		return nil, fmt.Errorf("redis: new client: %w", err)
	}

	client := goredis.NewClient(redisOpt)

	instance = &Pool{
		client: client,
		opt:    opts,
		closed: false,
	}

	return client, nil
}

// Close shuts down the shared Redis client. It is nil-safe and resets the singleton. It returns a wrapped close error if shutdown fails.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil || instance.client == nil {
		return nil
	}

	err := instance.client.Close()

	instance.closed = true
	instance = nil

	if err != nil {
		return fmt.Errorf("%w: %w", ErrCloseClient, err)
	}
	return nil
}
