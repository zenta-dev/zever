package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/internal/cas"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

type redisAdapter struct {
	client *goredis.Client
	closed atomic.Bool
}

var _ cache.CompareAndSwapCache = (*redisAdapter)(nil)

// connOptions maps cache options onto the shared client options. A set URL
// takes precedence over Addr; both spellings connect.
func connOptions(opts cache.Options) zredis.Options {
	addr := strings.TrimSpace(opts.URL)
	if addr == "" {
		addr = opts.Addr
	}

	return zredis.Options{
		Addr:            addr,
		Password:        opts.Password,
		DB:              opts.DB,
		TLS:             opts.TLS,
		RequireTLS:      opts.RequireTLS,
		PoolSize:        opts.PoolSize,
		MinIdleConns:    opts.MinIdleConns,
		PoolTimeout:     opts.PoolTimeout,
		ConnMaxIdleTime: opts.ConnMaxIdleTime,
		ConnMaxLifetime: opts.ConnMaxLifetime,
	}
}

// New creates a Redis-backed cache.Cache using a shared client from internal/redis, verifies connectivity with a 3s ping check, and reports failures with the redacted address in errors.
func New(opts cache.Options) (cache.Cache, error) {
	client, err := zredis.New(connOptions(opts))
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
	return zredis.RedactEndpoint(opts.URL, opts.Addr)
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

// CompareAndDelete removes key only when its value equals expected, using one
// Eval round trip. It reports deleted=false with nil error when the key is
// missing or holds another value, so callers never steal a successor entry.
func (a *redisAdapter) CompareAndDelete(ctx context.Context, key string, expected []byte) (bool, error) {
	if a.closed.Load() {
		return false, cache.ErrClosed
	}

	n, err := a.client.Eval(ctx, cas.CompareAndDeleteScript, []string{key}, expected).Int()
	if err != nil {
		return false, fmt.Errorf("cache: compare and delete %q error: %w", key, err)
	}

	return n == 1, nil
}

// CompareAndExtend renews the TTL on key only when its value equals expected,
// using one Eval round trip. A non-positive ttl clears the expiry via
// PERSIST. Missing or mismatched keys report extended=false with nil error.
func (a *redisAdapter) CompareAndExtend(ctx context.Context, key string, expected []byte, ttl time.Duration) (bool, error) {
	if a.closed.Load() {
		return false, cache.ErrClosed
	}

	var n int
	var err error

	if ttl > 0 {
		n, err = a.client.Eval(ctx, cas.CompareAndExpireScript, []string{key}, expected, ttl.Milliseconds()).Int()
	} else {
		n, err = a.client.Eval(ctx, cas.CompareAndPersistScript, []string{key}, expected).Int()
	}

	if err != nil {
		return false, fmt.Errorf("cache: compare and extend %q error: %w", key, err)
	}

	return n == 1, nil
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

	return zredis.Close(a.client)
}
