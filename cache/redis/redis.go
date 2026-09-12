package redis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/cache"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

type redisAdapter struct {
	client *goredis.Client
	closed atomic.Bool
}

// New creates a Redis-backed cache.Cache using a shared client from internal/redis, verifies connectivity with a 3s ping check, and reports failures with the redacted address in errors.
func New(opts cache.Options) (cache.Cache, error) {
	client, err := zredis.New(zredis.Options{
		URL:             opts.URL,
		Addr:            opts.Addr,
		Password:        opts.Password,
		DB:              opts.DB,
		PoolSize:        opts.PoolSize,
		MinIdleConns:    opts.MinIdleConns,
		PoolTimeout:     opts.PoolTimeout,
		ConnMaxIdleTime: opts.ConnMaxIdleTime,
		ConnMaxLifetime: opts.ConnMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("cache: connect %q error: %w", redactURL(opts), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("cache: ping %q error: %w", redactURL(opts), err)
	}

	return &redisAdapter{client: client}, nil
}

// redactURL returns opts.URL (or opts.Addr, if URL is empty) with any
// embedded userinfo credentials masked, suitable for inclusion in error
// messages.
func redactURL(opts cache.Options) string {
	raw := strings.TrimSpace(opts.URL)
	if raw == "" {
		raw = strings.TrimSpace(opts.Addr)
	}

	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}

	u.User = url.UserPassword(u.User.Username(), "xxxxx")

	return u.String()
}

func (a *redisAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	if a.closed.Load() {
		return nil, cache.ErrClosed
	}

	b, err := a.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, &cache.NotFoundError{Key: key}
	}

	if err != nil {
		return nil, fmt.Errorf("cache: get %q error: %w", key, err)
	}

	return b, nil
}

func (a *redisAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if a.closed.Load() {
		return cache.ErrClosed
	}

	if err := a.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set %q error: %w", key, err)
	}

	return nil
}

func (a *redisAdapter) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if a.closed.Load() {
		return false, cache.ErrClosed
	}

	ok, err := a.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("cache: set if absent %q: %w", key, err)
	}

	return ok, nil
}

func (a *redisAdapter) Delete(ctx context.Context, key string) error {
	if a.closed.Load() {
		return cache.ErrClosed
	}

	if err := a.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache: delete %q error: %w", key, err)
	}

	return nil
}

func (a *redisAdapter) Increment(ctx context.Context, key string) error {
	if a.closed.Load() {
		return cache.ErrClosed
	}

	if err := a.client.Incr(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache: increment %q error: %w", key, err)
	}

	return nil
}

func (a *redisAdapter) Decrement(ctx context.Context, key string) error {
	if a.closed.Load() {
		return cache.ErrClosed
	}

	if err := a.client.Decr(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache: decrement %q error: %w", key, err)
	}

	return nil
}

func (a *redisAdapter) Exists(ctx context.Context, key string) (bool, error) {
	if a.closed.Load() {
		return false, cache.ErrClosed
	}

	n, err := a.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache: exists %q: %w", key, err)
	}

	return n > 0, nil
}

func (a *redisAdapter) Close(_ context.Context) error {
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}

	return zredis.Close()
}
