package redis

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/ratelimit"
)

//go:embed scripts/allow.lua
var allowLua string

var (
	allowScript = goredis.NewScript(allowLua)

	// scriptMu guards allowScript so shape-override tests can swap the
	// script without racing parallel Allow calls (same class as the
	// stdout randRead fix: a bare global swap would race).
	scriptMu sync.RWMutex
)

// Compile-time check that limiter implements ratelimit.Limiter.
var _ ratelimit.Limiter = (*limiter)(nil)

// DefaultPingTimeout bounds the startup connectivity check.
const DefaultPingTimeout = 3 * time.Second

type limiter struct {
	client *goredis.Client
	prefix string
	rate   float64
	burst  int
	closed atomic.Bool
}

// New creates a Redis-backed ratelimit.Limiter with its own client from
// internal/redis. IdleTTL/SweepInterval are meaningless for Redis and are
// ignored. It verifies connectivity with a 3s ping check.
func New(opts ratelimit.Options) (ratelimit.Limiter, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("redis: invalid options: %w", err)
	}

	prefix := strings.TrimSpace(opts.Redis.Prefix)
	if prefix == "" {
		prefix = "ratelimit"
	}

	client, err := zredis.New(opts.Redis.Options)
	if err != nil {
		return nil, fmt.Errorf("redis: connect %q: %w", redactAddr(opts.Redis.Addr), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("redis: ping %q: %w", redactAddr(opts.Redis.Addr), err)
	}

	return &limiter{
		client: client,
		prefix: prefix,
		rate:   opts.Rate,
		burst:  opts.Burst,
	}, nil
}

// redactAddr masks any embedded userinfo credentials, suitable for error
// messages. The password from options is never included.
func redactAddr(addr string) string {
	return zredis.RedactAddr(addr)
}

func (l *limiter) redisKey(key string) string {
	return l.prefix + ":" + key
}

// Allow consumes tokens for key and reports the decision. Errors are for
// infrastructure failures only; the fail-open decision is left to the
// caller.
func (l *limiter) Allow(ctx context.Context, key string, tokens float64) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: %w", err)
	}

	if l.closed.Load() {
		return ratelimit.Decision{}, ratelimit.ErrClosed
	}

	if err := ratelimit.ValidateKey(key); err != nil {
		return ratelimit.Decision{}, fmt.Errorf("redis: %w", err)
	}

	if err := ratelimit.ValidateCost(tokens, float64(l.burst)); err != nil {
		return ratelimit.Decision{}, fmt.Errorf("redis: %w", err)
	}

	cost := tokens
	if cost > float64(l.burst) {
		cost = float64(l.burst)
	}

	now := float64(time.Now().UnixNano()) / 1e9

	scriptMu.RLock()
	res, err := allowScript.Run(ctx, l.client, []string{l.redisKey(key)}, l.burst, l.rate, now, cost).Result()
	scriptMu.RUnlock()
	if err != nil {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: %w", err)
	}

	vals, ok := res.([]any)
	if !ok || len(vals) != 3 {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: unexpected script result %T (%v)", res, res)
	}

	allowed, ok := vals[0].(int64)
	if !ok {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: unexpected allowed type %T", vals[0])
	}

	remaining, ok := vals[1].(int64)
	if !ok {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: unexpected remaining type %T", vals[1])
	}

	retryMS, ok := vals[2].(int64)
	if !ok {
		return ratelimit.Decision{}, fmt.Errorf("redis: allow: unexpected retry type %T", vals[2])
	}

	if retryMS < 0 {
		retryMS = 0
	}

	if allowed == 1 {
		return ratelimit.Decision{Allowed: true, Remaining: float64(remaining)}, nil
	}

	return ratelimit.Decision{
		Allowed:    false,
		RetryAfter: time.Duration(retryMS) * time.Millisecond,
		Remaining:  float64(remaining),
	}, nil
}

// Reset clears the bucket state for key; missing keys return nil.
func (l *limiter) Reset(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("redis: reset: %w", err)
	}

	if l.closed.Load() {
		return ratelimit.ErrClosed
	}

	if err := ratelimit.ValidateKey(key); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	if err := l.client.Del(ctx, l.redisKey(key)).Err(); err != nil {
		return fmt.Errorf("redis: reset: %w", err)
	}

	return nil
}

// Close marks the limiter closed and closes its underlying client; it is
// idempotent.
func (l *limiter) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}

	return zredis.Close(l.client)
}

// Name returns the adapter name.
func (l *limiter) Name() string { return "redis" }
