package redis

import (
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// New builds a new Redis client from opts. Each call returns an
// independently owned client; callers must Close the client they
// receive once they are done with it.
func New(opts Options) (*goredis.Client, error) {
	redisOpt, err := opts.toRedisOptions()
	if err != nil {
		return nil, fmt.Errorf("redis: new client: %w", err)
	}

	return goredis.NewClient(redisOpt), nil
}

// Close shuts down client. It is nil-safe. It returns a wrapped close
// error if shutdown fails.
func Close(client *goredis.Client) error {
	if client == nil {
		return nil
	}

	if err := client.Close(); err != nil {
		return fmt.Errorf("%w: %w", ErrCloseClient, err)
	}
	return nil
}
