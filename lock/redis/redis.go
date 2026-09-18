package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/internal/cas"
	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/lock"
)

// releaseScript deletes the key only when it still holds holder.
// It returns 1 on release, 0 when the key is missing or owned by another
// holder, so Unlock never steals someone else's lock. Shared with cache CAS
// via internal/cas to keep owner-check semantics identical.
const releaseScript = cas.CompareAndDeleteScript

// extendScript renews the key TTL only when it still holds holder.
// It returns 1 on renewal, 0 when the key is missing or owned by another
// holder. GET and PEXPIRE run atomically inside the script. Shared with
// cache CAS via internal/cas.
const extendScript = cas.CompareAndExpireScript

// redisClient is the subset of go-redis used by the adapter, faked in
// tests to drive client errors without a server.
type redisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *goredis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *goredis.Cmd
}

// adapter is a Redis-backed lock.Locker. It is safe for concurrent use;
// each acquired lock carries its own holder id.
type adapter struct {
	client        redisClient
	prefix        string
	ttl           time.Duration
	retryInterval time.Duration
	closed        atomic.Bool
}

// closeShared releases the shared zredis client. It is a seam so tests can
// inject a close failure; production always resets the singleton.
var closeShared = zredis.Close

// handle is one acquired lock.Lock.
type handle struct {
	a      *adapter
	client redisClient
	key    string
	holder string
}

// connOptions maps lock options onto the shared client options. A set URL
// takes precedence over Addr; both spellings connect.
func connOptions(opts lock.Options) zredis.Options {
	addr := strings.TrimSpace(opts.URL)
	if addr == "" {
		addr = opts.Addr
	}

	return zredis.Options{
		Addr:     addr,
		Password: opts.Password,
		DB:       opts.DB,
		TLS:      opts.TLS,
	}
}

// New creates a Redis-backed lock.Locker. Empty prefix, non-positive TTL
// and retry intervals fall back to "lock:", lock.DefaultTTL and
// lock.DefaultRetryInterval. It verifies connectivity with a 3s ping check.
func New(opts lock.Options) (lock.Locker, error) {
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = "lock:"
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = lock.DefaultTTL
	}

	retry := opts.RetryInterval
	if retry <= 0 {
		retry = lock.DefaultRetryInterval
	}

	client, err := zredis.New(connOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("[lock] connect %q error: %w", redactURL(opts), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Reset the singleton instead of closing the client directly: the
		// instance would otherwise keep pointing at a dead client and
		// poison the next New with different options.
		_ = closeShared()

		return nil, fmt.Errorf("[lock] ping %q error: %w", redactURL(opts), err)
	}

	return &adapter{
		client:        client,
		prefix:        prefix,
		ttl:           ttl,
		retryInterval: retry,
	}, nil
}

// redactURL returns opts.URL (or opts.Addr, if URL is empty) with any
// embedded userinfo credentials masked, suitable for inclusion in error
// messages and safe to log: secrets never appear in errors.
func redactURL(opts lock.Options) string {
	return zredis.RedactEndpoint(opts.URL, opts.Addr)
}

// randRead is a seam for newHolderID, stubbed in tests to force failure.
var randRead = rand.Read

// newHolderID returns a random hex holder id.
func newHolderID() (string, error) {
	var b [16]byte

	if _, err := randRead(b[:]); err != nil {
		return "", fmt.Errorf("[lock] holder id error: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}

// TryAcquire attempts to acquire key exactly once with SET NX PX, reporting
// ok=false with a nil error when the key is currently held. A non-positive
// ttl selects the adapter default.
func (a *adapter) TryAcquire(ctx context.Context, key string, ttl time.Duration) (lock.Lock, bool, error) {
	if key == "" {
		return nil, false, fmt.Errorf("[lock] TryAcquire %q error: key is empty", key)
	}

	if ttl <= 0 {
		ttl = a.ttl
	}

	holder, err := newHolderID()
	if err != nil {
		return nil, false, err
	}

	ok, err := a.client.SetNX(ctx, a.prefix+key, holder, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("[lock] TryAcquire %q error: %w", key, err)
	}

	if !ok {
		return nil, false, nil
	}

	return &handle{a: a, client: a.client, key: key, holder: holder}, true, nil
}

// Acquire blocks, retrying every retry interval, until the lock is acquired
// or ctx is done. It returns ctx.Err() wrapped if the context lapses first.
func (a *adapter) Acquire(ctx context.Context, key string, ttl time.Duration) (lock.Lock, error) {
	t := time.NewTimer(a.retryInterval)
	defer t.Stop()

	for {
		l, ok, err := a.TryAcquire(ctx, key, ttl)
		if err != nil {
			return nil, err
		}

		if ok {
			return l, nil
		}

		t.Reset(a.retryInterval)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("[lock] Acquire %q error: %w", key, ctx.Err())
		case <-t.C:
		}
	}
}

// Close releases the Redis connection via the shared-client reset. It is
// idempotent. Leases still held by callers are not released individually;
// they expire on their own TTL.
func (a *adapter) Close(_ context.Context) error {
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}

	return closeShared()
}

// Key returns the locked key.
func (h *handle) Key() string { return h.key }

// Extend renews the lease for a further ttl, or the adapter default when
// ttl is non-positive. It returns lock.ErrNotHeld when the lease was
// already lost or taken over by another holder.
func (h *handle) Extend(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = h.a.ttl
	}

	n, err := h.client.Eval(ctx, extendScript, []string{h.a.prefix + h.key}, h.holder, ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("[lock] Extend %q error: %w", h.key, err)
	}

	if n == 0 {
		return fmt.Errorf("[lock] Extend %q error: %w", h.key, lock.ErrNotHeld)
	}

	return nil
}

// Unlock releases the lease. It returns lock.ErrNotHeld when the lease was
// already lost or taken over, and never deletes a lease it no longer owns.
func (h *handle) Unlock(ctx context.Context) error {
	n, err := h.client.Eval(ctx, releaseScript, []string{h.a.prefix + h.key}, h.holder).Int()
	if err != nil {
		return fmt.Errorf("[lock] Unlock %q error: %w", h.key, err)
	}

	if n == 0 {
		return fmt.Errorf("[lock] Unlock %q error: %w", h.key, lock.ErrNotHeld)
	}

	return nil
}
