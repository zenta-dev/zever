package redis

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/idempotency"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

const (
	tagPending = 'P'
	tagDone    = 'D'

	// defaultPrefix namespaces idempotency keys when no prefix is set.
	defaultPrefix = "idem:"

	// beginAttempts bounds the retry loop for the (rare) case where a
	// reservation expires between our failed SET NX and the follow-up GET.
	beginAttempts = 3
)

// Compile-time check that store implements idempotency.Store.
var _ idempotency.Store = (*store)(nil)

type store struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
	closed atomic.Bool
}

// New creates a Redis-backed idempotency.Store using a shared client from
// internal/redis. It verifies connectivity with a 3s ping check and reports
// failures with the redacted address in errors. Close is idempotent and
// never closes the shared pool.
func New(opts idempotency.Options) (idempotency.Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("redis: invalid options: %w", err)
	}

	prefix := opts.Redis.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = idempotency.DefaultTTL
	}

	// zredis.New is infallible for validated options: Validate rejects
	// addresses with a scheme, so toRedisOptions takes the plain
	// host:port branch (which has no error source), and pool replacement
	// Close returns nil for these clients. Connectivity is still verified
	// by the ping check below, preserving fail-closed behavior.
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
	return zredis.RedactAddr(addr)
}

func (s *store) redisKey(key string) string {
	return s.prefix + key
}

// Begin atomically checks for an existing record and reserves key on miss:
// miss returns ({false, nil}, nil) and the caller must execute, then call
// Complete. A hit on a completed record with matching fingerprint returns
// ({true, copy}, nil). A hit on a pending record returns ErrInProgress.
// Fingerprint is checked FIRST on any existing record. All errors are
// fail-closed.
func (s *store) Begin(ctx context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", err)
	}

	if s.closed.Load() {
		return idempotency.Outcome{}, idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return idempotency.Outcome{}, fmt.Errorf("redis: %w", err)
	}

	if err := checkFingerprint(opts.Fingerprint); err != nil {
		return idempotency.Outcome{}, err
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}

	k := s.redisKey(key)

	for range beginAttempts {
		won, err := s.client.SetNX(ctx, k, encodePending(opts.Fingerprint), ttl).Result()
		if err != nil {
			return idempotency.Outcome{}, fmt.Errorf("redis: begin claim: %w", err)
		}

		if won {
			return idempotency.Outcome{}, nil
		}

		raw, err := s.client.Get(ctx, k).Bytes()
		if errors.Is(err, goredis.Nil) {
			// Winner expired mid-race; re-attempt the claim.
			continue
		}

		if err != nil {
			return idempotency.Outcome{}, fmt.Errorf("redis: begin get: %w", err)
		}

		tag, stored, result, err := decode(raw)
		if err != nil {
			// Never replay garbage: fail closed.
			return idempotency.Outcome{}, fmt.Errorf("redis: begin decode: %w", err)
		}

		if !idempotency.FingerprintMatches(stored, opts.Fingerprint) {
			return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", idempotency.ErrKeyMismatch)
		}

		if tag == tagPending {
			return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", idempotency.ErrInProgress)
		}

		out := idempotency.Outcome{Replay: true}
		if len(result) > 0 {
			out.Result = append([]byte(nil), result...)
		}

		return out, nil
	}

	return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", idempotency.ErrInProgress)
}

// Complete stores result for key and marks the record done. It reads the
// existing record first (2 RTTs) so a fingerprint mismatch reports
// ErrKeyMismatch without overwriting. A missing record (no prior Begin) is
// upserted under the store TTL. Completed records inherit the remaining TTL
// via KeepTTL; only an expired-missing record uses a fresh TTL. A corrupt
// record fails closed and is never overwritten.
func (s *store) Complete(ctx context.Context, key string, fingerprint, result []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("redis: complete: %w", err)
	}

	if s.closed.Load() {
		return idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	if err := checkFingerprint(fingerprint); err != nil {
		return err
	}

	k := s.redisKey(key)
	done := encodeDone(fingerprint, result)

	raw, err := s.client.Get(ctx, k).Bytes()
	if errors.Is(err, goredis.Nil) {
		if serr := s.client.Set(ctx, k, done, s.ttl).Err(); serr != nil {
			return fmt.Errorf("redis: complete set: %w", serr)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("redis: complete get: %w", err)
	}

	_, stored, _, err := decode(raw)
	if err != nil {
		return fmt.Errorf("redis: complete decode: %w", err)
	}

	if !idempotency.FingerprintMatches(stored, fingerprint) {
		return fmt.Errorf("redis: complete: %w", idempotency.ErrKeyMismatch)
	}

	// XX + KeepTTL overwrites the reservation in place, preserving the
	// deadline Begin set. If the reservation expired since the GET, fall
	// back to a fresh write under the store TTL rather than dropping
	// the result.
	setErr := s.client.SetArgs(ctx, k, done, goredis.SetArgs{Mode: "XX", KeepTTL: true}).Err()
	if setErr != nil {
		if errors.Is(setErr, goredis.Nil) {
			if ferr := s.client.Set(ctx, k, done, s.ttl).Err(); ferr != nil {
				return fmt.Errorf("redis: complete set: %w", ferr)
			}

			return nil
		}

		return fmt.Errorf("redis: complete set: %w", setErr)
	}

	return nil
}

// Forget removes key; missing keys return nil.
func (s *store) Forget(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("redis: forget: %w", err)
	}

	if s.closed.Load() {
		return idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	if err := s.client.Del(ctx, s.redisKey(key)).Err(); err != nil {
		return fmt.Errorf("redis: forget: %w", err)
	}

	return nil
}

// Close marks the store closed; it is idempotent and never closes the
// shared pool.
func (s *store) Close() error {
	s.closed.Store(true)

	return nil
}

// maxFingerprintLen caps per-call fingerprints: hashes are tiny, wire
// length is 2 bytes, and unbounded values would abuse store memory.
const maxFingerprintLen = 4096

func checkFingerprint(fp []byte) error {
	if len(fp) > maxFingerprintLen {
		return fmt.Errorf("redis: fingerprint %d bytes exceeds %d: %w", len(fp), maxFingerprintLen, idempotency.ErrFingerprintTooLarge)
	}

	return nil
}

// encodePending builds a pending wire record: tag + fpLen + fingerprint.
func encodePending(fp []byte) []byte {
	out := make([]byte, 0, 3+len(fp))
	out = append(out, tagPending, byte((len(fp)>>8)&0xff), byte(len(fp)&0xff))

	return append(out, fp...)
}

// encodeDone builds a completed wire record: tag + fpLen + fingerprint +
// result.
func encodeDone(fp, result []byte) []byte {
	out := make([]byte, 0, 3+len(fp)+len(result))
	out = append(out, tagDone, byte((len(fp)>>8)&0xff), byte(len(fp)&0xff))
	out = append(out, fp...)

	return append(out, result...)
}

// decode splits a wire record into tag, fingerprint, and result.
// Result aliases raw; callers needing ownership must copy.
func decode(raw []byte) (tag byte, fp, result []byte, err error) {
	if len(raw) < 3 {
		return 0, nil, nil, fmt.Errorf("%w: record too short: %d bytes", idempotency.ErrCorruptRecord, len(raw))
	}

	tag = raw[0]
	if tag != tagPending && tag != tagDone {
		return 0, nil, nil, fmt.Errorf("%w: unknown tag %q", idempotency.ErrCorruptRecord, tag)
	}

	n := int(binary.BigEndian.Uint16(raw[1:3]))
	if len(raw) < 3+n {
		return 0, nil, nil, fmt.Errorf("%w: fingerprint length %d exceeds record %d bytes", idempotency.ErrCorruptRecord, n, len(raw))
	}

	fp = raw[3 : 3+n]
	result = raw[3+n:]

	return tag, fp, result, nil
}
