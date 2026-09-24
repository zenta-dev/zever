// Package redis provides a Redis-backed revocation.Store, so revoked jtis
// are visible across every instance sharing the same Redis and survive
// process restarts. Each revoked jti is stored as a key with a TTL matching
// its remaining validity, so Redis itself expires the entry: no background
// pruner is needed.
package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/auth/jwt/revocation"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

// defaultPrefix namespaces revocation keys when no prefix is set.
const defaultPrefix = "jwtrevoke:"

// DefaultPingTimeout bounds the startup connectivity check.
const DefaultPingTimeout = 3 * time.Second

// revokedValue is the value stored for a revoked key; only key existence is
// consulted, so the value itself carries no meaning.
const revokedValue = "1"

var _ revocation.Store = (*store)(nil)

// Options configures the Redis-backed revocation.Store.
type Options struct {
	// ConnectOptions holds the shared Redis connection settings.
	zredis.ConnectOptions
	// URL is the Redis connection URL. When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`
	// Prefix scopes revocation keys to one namespace. Empty uses defaultPrefix.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

type store struct {
	client *goredis.Client
	prefix string
	closed atomic.Bool
}

// connOptions maps Options onto the shared client options. A set URL takes
// precedence over Addr; both spellings connect.
func connOptions(opts Options) zredis.Options {
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

// New creates a Redis-backed revocation.Store with its own internal/redis
// client. Empty prefix falls back to defaultPrefix. It verifies connectivity
// with a 3s ping check.
func New(opts Options) (revocation.Store, error) {
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = defaultPrefix
	}

	client, err := zredis.New(connOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("revocation/redis: connect %q: %w", redactURL(opts), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = zredis.Close(client)

		return nil, fmt.Errorf("revocation/redis: ping %q: %w", redactURL(opts), err)
	}

	return &store{client: client, prefix: prefix}, nil
}

// redactURL returns opts.URL (or opts.Addr, if URL is empty) with any
// embedded userinfo credentials masked, safe for inclusion in errors.
func redactURL(opts Options) string {
	return zredis.RedactEndpoint(opts.URL, opts.Addr)
}

func (s *store) key(jti string) string {
	return s.prefix + jti
}

// Revoke marks jti as revoked with a TTL derived from until, so Redis
// expires the entry on its own. An until that has already lapsed is a
// no-op: there is nothing left to enforce, and Redis rejects a non-positive
// expiration outright. Callers that want a grace window for already-expired
// tokens (as auth/jwt.Adapter.Revoke does) must pass an until floored in
// the future themselves.
func (s *store) Revoke(ctx context.Context, jti string, until time.Time) error {
	if jti == "" {
		return errors.New("revocation/redis: revoke: empty jti")
	}
	if s.closed.Load() {
		return revocation.ErrClosed
	}

	ttl := time.Until(until)
	if ttl <= 0 {
		return nil
	}

	if err := s.client.Set(ctx, s.key(jti), revokedValue, ttl).Err(); err != nil {
		return fmt.Errorf("revocation/redis: revoke: %w", err)
	}

	return nil
}

// IsRevoked reports whether jti is currently revoked.
func (s *store) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	if s.closed.Load() {
		return false, revocation.ErrClosed
	}

	n, err := s.client.Exists(ctx, s.key(jti)).Result()
	if err != nil {
		return false, fmt.Errorf("revocation/redis: isrevoked: %w", err)
	}

	return n > 0, nil
}

// Close releases the store's own Redis connection. It is idempotent.
func (s *store) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	return zredis.Close(s.client)
}
