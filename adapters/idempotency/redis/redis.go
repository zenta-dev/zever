package redis

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/idempotency"
	redisclient "github.com/zenta-dev/zever/shared/redisclient"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

const (
	tagPending = 'P'
	tagDone    = 'D'

	// defaultPrefix namespaces idempotency keys when no prefix is set.
	defaultPrefix = "idem:"
)

// DefaultPingTimeout bounds the startup connectivity check.
const DefaultPingTimeout = 3 * time.Second

// Compile-time check that store implements idempotency.Store.
var _ idempotency.Store = (*store)(nil)

type store struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
	closed atomic.Bool
}

// New creates a Redis-backed idempotency.Store with its own client from
// internal/redis. It verifies connectivity with a 3s ping check and reports
// failures with the redacted address in errors. Close is idempotent and
// closes the store's own client.
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

	client, err := redisclient.New(opts.Redis.Options)
	if err != nil {
		return nil, fmt.Errorf("redis: connect %q: %w", redactAddr(opts.Redis.Addr), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
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
	return redisopt.RedactAddr(addr)
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
	mode, ttlVal := ttlModeVal(ttl)

	res, err := beginScript.Run(ctx, s.client, []string{k}, encodePending(opts.Fingerprint), mode, ttlVal, opts.Fingerprint).Result()
	if err != nil {
		return idempotency.Outcome{}, fmt.Errorf("redis: begin claim: %w", err)
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 3 {
		return idempotency.Outcome{}, fmt.Errorf("redis: begin: unexpected result %v", res)
	}

	replay := asBool(arr[0])
	result, _ := arr[1].(string)

	switch status, _ := arr[2].(string); status {
	case "in_progress":
		return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", idempotency.ErrInProgress)
	case "mismatch":
		return idempotency.Outcome{}, fmt.Errorf("redis: begin: %w", idempotency.ErrKeyMismatch)
	case "decode":
		return idempotency.Outcome{}, fmt.Errorf("redis: begin decode: %w", idempotency.ErrCorruptRecord)
	}

	out := idempotency.Outcome{Replay: replay}
	if replay && len(result) > 0 {
		out.Result = append([]byte(nil), result...)
	}

	return out, nil
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
	mode, ttlVal := ttlModeVal(s.ttl)

	res, err := completeScript.Run(ctx, s.client, []string{k}, encodeDone(fingerprint, result), mode, ttlVal, fingerprint).Result()
	if err != nil {
		return fmt.Errorf("redis: complete: %w", err)
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 1 {
		return fmt.Errorf("redis: complete: unexpected result %v", res)
	}

	switch status, _ := arr[0].(string); status {
	case "mismatch":
		return fmt.Errorf("redis: complete: %w", idempotency.ErrKeyMismatch)
	case "decode":
		return fmt.Errorf("redis: complete decode: %w", idempotency.ErrCorruptRecord)
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

// Close marks the store closed and closes its underlying client; it is
// idempotent.
func (s *store) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	return redisclient.Close(s.client)
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

// asBool normalizes a Lua table element to a bool. Redis converts a Lua
// boolean nested in a table to an integer reply, so the script's replay flag
// can arrive as either a bool or an int64 depending on the reply path.
func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	default:
		return false
	}
}

// ttlModeVal replicates go-redis's SET expiration formatting so the Lua
// scripts set exactly the duration the previous client calls would: a
// duration that is not a whole number of seconds (or is sub-second) uses PX
// with milliseconds, otherwise EX with seconds. The sub-millisecond case
// truncates to 1ms, matching go-redis's formatMs.
func ttlModeVal(ttl time.Duration) (string, int64) {
	if ttl < time.Second || ttl%time.Second != 0 {
		ms := ttl.Milliseconds()
		if ttl > 0 && ms == 0 {
			ms = 1
		}

		return "PX", ms
	}

	return "EX", int64(ttl / time.Second)
}

// encodePending builds a pending wire record: tag + fpLen + fingerprint.
func encodePending(fp []byte) []byte {
	if len(fp) > math.MaxInt-3 {
		panic("redis: encodePending size overflow")
	}
	size := 3 + len(fp)

	out := make([]byte, 0, size)
	out = append(out, tagPending, byte((len(fp)>>8)&0xff), byte(len(fp)&0xff))

	return append(out, fp...)
}

// encodeDone builds a completed wire record: tag + fpLen + fingerprint +
// result.
func encodeDone(fp, result []byte) []byte {
	if len(fp) > math.MaxInt-3 {
		panic("redis: encodeDone size overflow")
	}
	base := 3 + len(fp)
	if len(result) > math.MaxInt-base {
		panic("redis: encodeDone size overflow")
	}
	size := base + len(result)

	out := make([]byte, 0, size)
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
