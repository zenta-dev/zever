package memory

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
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

// janitorInterval is how often the background goroutine reaps expired
// records. Expired records are also reaped lazily on access.
var janitorInterval = time.Minute

type store struct {
	mu         sync.Mutex
	entries    map[string]*entry
	prefix     string
	ttl        time.Duration
	maxEntries int
	closed     atomic.Bool
	stop       chan struct{}
	done       chan struct{}
}

// New creates an in-memory idempotency Store from opts. Non-positive
// MaxEntries resolves to idempotency.DefaultMaxEntries; past the bound a
// new key evicts the soonest-expire live record. A background janitor
// reaps expired records periodically and is joined on Close.
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

	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = idempotency.DefaultMaxEntries
	}

	s := &store{
		entries:    make(map[string]*entry),
		prefix:     prefix,
		ttl:        ttl,
		maxEntries: maxEntries,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}

	go s.run()

	return s, nil
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

	// The live record for k (if any) is expired and will be replaced;
	// evict before inserting when the table is full.
	if _, ok := s.entries[k]; ok {
		delete(s.entries, k)
	} else {
		s.evictLocked(now)
	}

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

	if _, ok := s.entries[k]; ok {
		delete(s.entries, k)
	} else {
		s.evictLocked(now)
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

// Close shuts down the store, stops the janitor, and releases resources.
func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.stop)
	<-s.done

	s.mu.Lock()
	s.entries = make(map[string]*entry)
	s.mu.Unlock()

	return nil
}

// evictLRULocked makes room for one new record when the table is full.
// Expired records are reaped first; otherwise the soonest-expire live
// record is evicted so the map cannot grow without bound. Caller must
// hold s.mu.
func (s *store) evictLocked(now time.Time) {
	if len(s.entries) < s.maxEntries {
		return
	}

	var victimKey string
	var victimExpiry time.Time
	first := true

	for k, e := range s.entries {
		if e.expired(now) {
			delete(s.entries, k)

			return
		}

		if first || e.expires.Before(victimExpiry) {
			victimKey, victimExpiry, first = k, e.expires, false
		}
	}

	if victimKey != "" {
		delete(s.entries, victimKey)
	}
}

func (s *store) run() {
	t := time.NewTicker(janitorInterval)
	defer t.Stop()
	defer close(s.done)

	for {
		select {
		case <-t.C:
			s.sweep()
		case <-s.stop:
			return
		}
	}
}

func (s *store) sweep() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, e := range s.entries {
		if e.expired(now) {
			delete(s.entries, k)
		}
	}
}
