package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/idempotency"
)

var _ idempotency.Store = (*store)(nil)

type entry struct {
	fp      []byte
	result  []byte
	done    bool
	expires time.Time
}

func (e *entry) expired(now time.Time) bool {
	return now.After(e.expires)
}

type store struct {
	mu      sync.Mutex
	entries map[string]*entry
	prefix  string
	ttl     time.Duration
	closed  atomic.Bool
}

func (s *store) sweepLocked(now time.Time) {
	for k, e := range s.entries {
		if e.expired(now) {
			delete(s.entries, k)
		}
	}
}

// New creates an in-memory idempotency Store from opts.
func New(opts idempotency.Options) (idempotency.Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = idempotency.DefaultTTL
	}

	prefix := opts.Redis.Prefix
	if prefix == "" {
		prefix = "idem:"
	}

	return &store{
		entries: make(map[string]*entry),
		prefix:  prefix,
		ttl:     ttl,
	}, nil
}

// maxFingerprintLen caps per-call fingerprints: hashes are tiny and
// unbounded values would abuse store memory.
const maxFingerprintLen = 4096

func checkFingerprint(fp []byte) error {
	if len(fp) > maxFingerprintLen {
		return fmt.Errorf("memory: fingerprint %d bytes exceeds %d: %w", len(fp), maxFingerprintLen, idempotency.ErrFingerprintTooLarge)
	}

	return nil
}

// Begin checks for an existing record and reserves key on miss.
func (s *store) Begin(ctx context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Outcome{}, err
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return idempotency.Outcome{}, fmt.Errorf("memory: %w", err)
	}

	if err := checkFingerprint(opts.Fingerprint); err != nil {
		return idempotency.Outcome{}, err
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return idempotency.Outcome{}, idempotency.ErrClosed
	}

	// Opportunistic full-map expiry purge. This is O(n) on every Begin,
	// which is acceptable for process-local use; beyond ~10k keys prefer
	// the redis adapter.
	s.sweepLocked(now)

	k := s.prefix + key

	if e, ok := s.entries[k]; ok && !e.expired(now) {
		if !idempotency.FingerprintMatches(e.fp, opts.Fingerprint) {
			return idempotency.Outcome{}, idempotency.ErrKeyMismatch
		}

		if !e.done {
			return idempotency.Outcome{}, fmt.Errorf("memory: %w", idempotency.ErrInProgress)
		}

		return idempotency.Outcome{Replay: true, Result: slices.Clone(e.result)}, nil
	}

	// No expired-entry cleanup here: sweepLocked above ran under the same
	// lock with the same now, so nothing expired can remain.
	s.entries[k] = &entry{fp: slices.Clone(opts.Fingerprint), expires: now.Add(ttl)}

	return idempotency.Outcome{}, nil
}

// Complete stores result for key and marks the record done.
func (s *store) Complete(ctx context.Context, key string, fingerprint, result []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("memory: %w", err)
	}

	if err := checkFingerprint(fingerprint); err != nil {
		return err
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return idempotency.ErrClosed
	}

	k := s.prefix + key

	if e, ok := s.entries[k]; ok && !e.expired(now) {
		if !idempotency.FingerprintMatches(e.fp, fingerprint) {
			return idempotency.ErrKeyMismatch
		}

		e.fp = slices.Clone(fingerprint)
		e.result = slices.Clone(result)
		e.done = true

		return nil
	}

	s.entries[k] = &entry{
		fp:      slices.Clone(fingerprint),
		result:  slices.Clone(result),
		done:    true,
		expires: now.Add(s.ttl),
	}

	return nil
}

// Forget removes key; missing keys return nil.
func (s *store) Forget(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("memory: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return idempotency.ErrClosed
	}

	delete(s.entries, s.prefix+key)

	return nil
}

// Close shuts down the store and releases associated resources.
func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.mu.Lock()
	s.entries = make(map[string]*entry)
	s.mu.Unlock()

	return nil
}
