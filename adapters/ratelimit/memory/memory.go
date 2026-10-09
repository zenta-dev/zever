package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/ratelimit"
)

// DefaultStripeCount is the number of lock stripes the bucket table is
// split into. It must be a power of two so stripe selection is a mask.
// Concurrent calls on different keys proceed in parallel; same-key calls
// still serialize on their stripe.
const DefaultStripeCount = 64

var _ ratelimit.Limiter = (*store)(nil)

type bucket struct {
	tokens float64
	last   time.Time
}

// stripe is one shard of the bucket table with its own lock.
type stripe struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
}

type store struct {
	stripes    []stripe
	total      atomic.Int64
	admitMu    sync.Mutex
	rate       float64
	burst      float64
	idle       time.Duration
	sweep      time.Duration
	maxEntries int
	closed     atomic.Bool
	stop       chan struct{}
	done       chan struct{}
}

// New creates an in-process token-bucket limiter from opts.
// Zero IdleTTL/SweepInterval resolve to ratelimit defaults. Non-positive
// MaxEntries resolves to ratelimit.DefaultMaxEntries; past the bound a new
// key evicts the least-recently-used live bucket.
func New(opts ratelimit.Options) (ratelimit.Limiter, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	idle := opts.IdleTTL
	if idle <= 0 {
		idle = ratelimit.DefaultIdleTTL
	}

	sweep := opts.SweepInterval
	if sweep <= 0 {
		sweep = ratelimit.DefaultSweepInterval
	}

	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = ratelimit.DefaultMaxEntries
	}

	s := &store{
		stripes:    make([]stripe, DefaultStripeCount),
		rate:       opts.Rate,
		burst:      float64(opts.Burst),
		idle:       idle,
		sweep:      sweep,
		maxEntries: maxEntries,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}

	for i := range s.stripes {
		s.stripes[i].buckets = make(map[string]*bucket)
	}

	go s.run()

	return s, nil
}

// Allow consumes tokens for key and reports the decision.
//
// Costs above burst are capped at burst for parity with the Redis adapter.
// bucket.last is touched on every call (allowed and denied) so idle
// expiry measures time since last activity, not last success.
//
// The bucket table is striped: existing-key calls take only their stripe
// lock. New-key calls serialize on admitMu across reserve-and-insert so at
// most one reservation is ever in flight; otherwise a burst of concurrent
// reservations is invisible to eviction and the MaxEntries bound can be
// overshot. Lock order is always admitMu before stripe locks.
func (s *store) Allow(ctx context.Context, key string, tokens float64) (ratelimit.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ratelimit.Decision{}, err
	}

	if err := ratelimit.ValidateKey(key); err != nil {
		return ratelimit.Decision{}, fmt.Errorf("memory: %w", err)
	}

	if err := ratelimit.ValidateCost(tokens, s.burst); err != nil {
		return ratelimit.Decision{}, fmt.Errorf("memory: %w", err)
	}

	if s.closed.Load() {
		return ratelimit.Decision{}, ratelimit.ErrClosed
	}

	// Cap oversized costs at burst (documented parity behavior).
	need := min(tokens, s.burst)

	now := time.Now()

	st := s.stripeFor(key)

	st.mu.Lock()

	if s.closed.Load() {
		st.mu.Unlock()
		return ratelimit.Decision{}, ratelimit.ErrClosed
	}

	b, ok := st.buckets[key]
	if !ok {
		st.mu.Unlock()

		var err error
		var fresh bool

		b, fresh, err = s.admitAndInsert(st, key, now)
		if err != nil {
			return ratelimit.Decision{}, err
		}

		// admitAndInsert returns with the stripe locked. A fresh bucket
		// skips the idle-expiry path below; a duplicate-found bucket
		// keeps the same check it always had.
		ok = !fresh
	}

	if ok && now.Sub(b.last) > s.idle {
		// Opportunistic expiry: treat idle bucket as new.
		b.tokens = s.burst
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	b.tokens = min(s.burst, b.tokens+elapsed*s.rate)
	b.last = now

	var d ratelimit.Decision

	if b.tokens >= need {
		b.tokens -= need
		d = ratelimit.Decision{Allowed: true, Remaining: b.tokens}
	} else {
		retry := time.Duration((need - b.tokens) / s.rate * float64(time.Second))
		d = ratelimit.Decision{Allowed: false, RetryAfter: retry, Remaining: b.tokens}
	}

	st.mu.Unlock()

	return d, nil
}

// Reset clears the bucket state for key. Unknown keys are a no-op.
func (s *store) Reset(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := ratelimit.ValidateKey(key); err != nil {
		return fmt.Errorf("memory: %w", err)
	}

	if s.closed.Load() {
		return ratelimit.ErrClosed
	}

	st := s.stripeFor(key)

	st.mu.Lock()
	defer st.mu.Unlock()

	if s.closed.Load() {
		return ratelimit.ErrClosed
	}

	if _, ok := st.buckets[key]; ok {
		delete(st.buckets, key)
		s.total.Add(-1)
	}

	return nil
}

// Close stops the background sweeper, wipes state, and is idempotent.
func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.stop)
	<-s.done

	// Hold admitMu so no admission lands between the wipe and the
	// total reset; stripe locks serialize against concurrent callers.
	s.admitMu.Lock()
	defer s.admitMu.Unlock()

	for i := range s.stripes {
		st := &s.stripes[i]
		st.mu.Lock()
		st.buckets = make(map[string]*bucket)
		st.mu.Unlock()
	}

	s.total.Store(0)

	return nil
}

// Name returns the adapter name.
func (s *store) Name() string { return "memory" }

func (s *store) run() {
	t := time.NewTicker(s.sweep)
	defer t.Stop()
	defer close(s.done)

	for {
		select {
		case <-t.C:
			s.sweepOnce()
		case <-s.stop:
			return
		}
	}
}

// stripeFor returns the stripe owning key.
func (s *store) stripeFor(key string) *stripe {
	return &s.stripes[fnv64a(key)&(DefaultStripeCount-1)]
}

// admitAndInsert reserves a slot under the global MaxEntries bound and
// inserts the bucket for key, returning with st locked, the bucket set,
// and fresh reporting whether this call created it (false when a
// concurrent caller won the race and their bucket is reused).
// The whole reserve-and-insert holds admitMu so no second reservation can
// go in flight: eviction always sees every prior insert, which keeps the
// bound strict under concurrency. A duplicate insert by a concurrent caller
// releases the reservation instead.
//
// Lock order is admitMu before stripe locks (Close follows the same order),
// so this cannot deadlock against eviction or the background sweeper.
func (s *store) admitAndInsert(st *stripe, key string, now time.Time) (*bucket, bool, error) {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()

	if s.total.Load() >= int64(s.maxEntries) {
		s.evictOneLocked(now)
	}

	s.total.Add(1)

	st.mu.Lock()

	if s.closed.Load() {
		s.total.Add(-1)
		st.mu.Unlock()
		return nil, false, ratelimit.ErrClosed
	}

	if b, ok := st.buckets[key]; ok {
		// A concurrent caller inserted this key while it was missing;
		// release the reservation and use their bucket.
		s.total.Add(-1)
		return b, false, nil
	}

	b := &bucket{tokens: s.burst, last: now}
	st.buckets[key] = b

	return b, true, nil
}

// evictOneLocked makes room for one new bucket when the table is full.
// Idle buckets are reaped first; otherwise the least-recently-used live
// bucket is evicted so an attacker cannot grow the map without bound.
// Caller must hold admitMu; total is decremented per deletion.
func (s *store) evictOneLocked(now time.Time) {
	var oldestKey string
	var oldestStripe *stripe
	var oldestTime time.Time
	first := true

	for i := range s.stripes {
		st := &s.stripes[i]

		st.mu.Lock()

		for k, b := range st.buckets {
			if now.Sub(b.last) > s.idle {
				delete(st.buckets, k)
				s.total.Add(-1)
				st.mu.Unlock()
				return
			}

			if first || b.last.Before(oldestTime) {
				oldestKey, oldestStripe, oldestTime, first = k, st, b.last, false
			}
		}

		st.mu.Unlock()
	}

	if oldestKey != "" {
		oldestStripe.mu.Lock()
		if _, ok := oldestStripe.buckets[oldestKey]; ok {
			delete(oldestStripe.buckets, oldestKey)
			s.total.Add(-1)
		}
		oldestStripe.mu.Unlock()
	}
}

func (s *store) sweepOnce() {
	now := time.Now()

	for i := range s.stripes {
		st := &s.stripes[i]

		st.mu.RLock()

		var expired []string

		for k, b := range st.buckets {
			if now.Sub(b.last) > s.idle {
				expired = append(expired, k)
			}
		}

		st.mu.RUnlock()

		if len(expired) == 0 {
			continue
		}

		st.mu.Lock()

		for _, k := range expired {
			if b, ok := st.buckets[k]; ok && now.Sub(b.last) > s.idle {
				delete(st.buckets, k)
				s.total.Add(-1)
			}
		}

		st.mu.Unlock()
	}
}

// fnv64a hashes key into a stripe index. FNV-1a is cheap and distributes
// typical limiter keys (IPs, user IDs) well enough for striping.
func fnv64a(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)

	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}
