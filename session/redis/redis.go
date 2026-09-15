// Package redis provides a Redis-backed session.Store.
//
// Sessions are stored as a single JSON value per key (prefix + ":" + id).
// Redis enforces absolute expiry at write time via SET EX: unlike the memory
// adapter's background sweeper, no access is needed for a session to expire.
// Save preserves the original absolute ExpiresAt with a read-then-write
// (GET + SET) that is NOT atomic: concurrent saves are last-write-wins, the
// same as the memory adapter. Corrupt records fail closed and are never
// replayed; Save overwrites them with a fresh expiry.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/session"
)

// defaultPrefix namespaces session keys when no prefix is set.
const defaultPrefix = "sess"

// Compile-time check that store implements session.Store.
var _ session.Store = (*store)(nil)

// wireSession is the JSON value stored per key. Times are Unix nanoseconds
// (0 means zero time); Data round-trips through JSON, so numbers decode as
// float64 and fresh maps are produced on every read (ownership is clean).
type wireSession struct {
	Data      map[string]any `json:"data"`
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
	ExpiresAt int64          `json:"expires_at"`
}

type store struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
	closed atomic.Bool
}

// New creates a Redis-backed session.Store using a shared client from
// internal/redis. It verifies connectivity with a 3s ping check and reports
// failures with the redacted address in errors. Close is idempotent and
// never closes the shared pool.
func New(opts session.Options) (session.Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	prefix := opts.Redis.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = session.DefaultTTL
	}

	// Validated options cannot fail Pool construction: toRedisOptions
	// errors only on redis:///rediss:// URLs (bad parse or missing host),
	// and Options.Validate rejects any address containing "://". The
	// client is built without I/O; the 3s Ping check below still verifies
	// connectivity, so a silent-nil client cannot slip through.
	client, _ := zredis.New(zredis.Options{
		Addr:     opts.Redis.Addr,
		Password: opts.Redis.Password,
		DB:       opts.Redis.DB,
		TLS:      opts.Redis.TLS,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("redis: ping %q: %w", redactAddr(opts.Redis.Addr), err)
	}

	return &store{client: client, prefix: prefix, ttl: ttl}, nil
}

// redactAddr masks any embedded userinfo credentials, suitable for error
// messages. The password from options is never included.
func redactAddr(addr string) string {
	raw := strings.TrimSpace(addr)

	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}

	u.User = url.UserPassword(u.User.Username(), "xxxxx")

	return u.String()
}

func (s *store) key(id string) string {
	return s.prefix + ":" + id
}

func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}

	return t.UnixNano()
}

func fromNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}

	return time.Unix(0, n).UTC()
}

func toWire(sess session.Session) wireSession {
	return wireSession{
		Data:      sess.Data,
		CreatedAt: unixNano(sess.CreatedAt),
		UpdatedAt: unixNano(sess.UpdatedAt),
		ExpiresAt: unixNano(sess.ExpiresAt),
	}
}

func fromWire(id string, w wireSession) session.Session {
	return session.Session{
		ID:        id,
		Data:      w.Data,
		CreatedAt: fromNano(w.CreatedAt),
		UpdatedAt: fromNano(w.UpdatedAt),
		ExpiresAt: fromNano(w.ExpiresAt),
	}
}

// Create mints a fresh session with the given ttl (<= 0 means the store
// default). Expiry is enforced by Redis at write time via SET EX.
func (s *store) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	if err := ctx.Err(); err != nil {
		return session.Session{}, err
	}

	if s.closed.Load() {
		return session.Session{}, session.ErrClosed
	}

	if ttl <= 0 {
		ttl = s.ttl
	}

	sess := session.NewSession(session.NewID(), ttl)

	// wireSession is {map, int64 x3}: encoding/json cannot fail on it
	// (nil maps, integers), so the error is provably infallible and
	// discarded.
	buf, _ := json.Marshal(toWire(sess))

	if err := s.client.Set(ctx, s.key(sess.ID), buf, ttl).Err(); err != nil {
		return session.Session{}, fmt.Errorf("redis: create set: %w", err)
	}

	return sess.Clone(), nil
}

// Get returns the session for id, or ErrNotFound on miss or expiry (expired
// sessions are indistinguishable from missing ones; the expired key is
// deleted best-effort). A corrupt record fails closed with a wrapped error
// and is never replayed.
func (s *store) Get(ctx context.Context, id string) (session.Session, error) {
	if err := ctx.Err(); err != nil {
		return session.Session{}, err
	}

	if err := session.ValidateID(id); err != nil {
		return session.Session{}, fmt.Errorf("redis: %w", err)
	}

	if s.closed.Load() {
		return session.Session{}, session.ErrClosed
	}

	raw, err := s.client.Get(ctx, s.key(id)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return session.Session{}, session.ErrNotFound
	}

	if err != nil {
		return session.Session{}, fmt.Errorf("redis: get: %w", err)
	}

	var w wireSession
	if err := json.Unmarshal(raw, &w); err != nil {
		// Best-effort cleanup like the expired path below; the error
		// itself stays generic-shaped (wrapped decode) without payload.
		_ = s.client.Del(ctx, s.key(id)).Err()

		return session.Session{}, fmt.Errorf("redis: decode: %w", err)
	}

	sess := fromWire(id, w)

	if sess.Expired(time.Now()) {
		// Best-effort cleanup; expiry itself is reported generically so
		// expired sessions are no oracle for key existence.
		_ = s.client.Del(ctx, s.key(id)).Err()

		return session.Session{}, session.ErrNotFound
	}

	return sess, nil
}

// Save upserts sess, touches UpdatedAt, and preserves the original absolute
// ExpiresAt for live records (never extends it). A missing, expired, or
// corrupt record is written fresh under the store default TTL, ignoring any
// caller-provided ExpiresAt. Read-modify-write is NOT atomic:
// last-write-wins under concurrency.
func (s *store) Save(ctx context.Context, sess session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := session.ValidateID(sess.ID); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	if s.closed.Load() {
		return session.ErrClosed
	}

	now := time.Now()
	createdAt := now
	ttl := s.ttl
	expiresAt := now.Add(ttl)

	raw, err := s.client.Get(ctx, s.key(sess.ID)).Bytes()
	switch {
	case errors.Is(err, goredis.Nil):
		// Missing: fresh write under the store default; defaults stand.
	case err != nil:
		return fmt.Errorf("redis: save get: %w", err)
	default:
		var existing wireSession
		if derr := json.Unmarshal(raw, &existing); derr != nil {
			// Corrupt existing record: overwrite fresh (fail-closed
			// forward); defaults stand.
			break
		}

		cur := fromWire(sess.ID, existing)
		if cur.Expired(now) {
			// Expired: treat as missing; defaults stand.
			break
		}

		createdAt = cur.CreatedAt
		if cur.ExpiresAt.IsZero() {
			// No absolute expiry recorded: persist without expiry.
			ttl = 0
			expiresAt = time.Time{}
			break
		}

		ttl = max(cur.ExpiresAt.Sub(now), time.Second)
		expiresAt = cur.ExpiresAt
	}

	buf, err := json.Marshal(wireSession{
		Data:      sess.Data,
		CreatedAt: unixNano(createdAt),
		UpdatedAt: unixNano(now),
		ExpiresAt: unixNano(expiresAt),
	})
	if err != nil {
		return fmt.Errorf("redis: encode: %w", err)
	}

	if err := s.client.Set(ctx, s.key(sess.ID), buf, ttl).Err(); err != nil {
		return fmt.Errorf("redis: save set: %w", err)
	}

	return nil
}

// Delete removes id; missing IDs return nil.
func (s *store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := session.ValidateID(id); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	if s.closed.Load() {
		return session.ErrClosed
	}

	if err := s.client.Del(ctx, s.key(id)).Err(); err != nil {
		return fmt.Errorf("redis: delete: %w", err)
	}

	return nil
}

// Close marks the store closed; it is idempotent and never closes the
// shared pool.
func (s *store) Close() error {
	s.closed.Store(true)

	return nil
}
