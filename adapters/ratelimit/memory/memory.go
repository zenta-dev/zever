package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/ratelimit"
)

var _ ratelimit.Limiter = (*store)(nil)

type bucket struct {
	tokens float64
	last   time.Time
}

type store struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
	idle    time.Duration
	sweep   time.Duration
	closed  atomic.Bool
	stop    chan struct{}
	done    chan struct{}
}

// New creates an in-process token-bucket limiter from opts.
// Zero IdleTTL/SweepInterval resolve to ratelimit defaults.
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

	s := &store{
		buckets: make(map[string]*bucket),
		rate:    opts.Rate,
		burst:   float64(opts.Burst),
		idle:    idle,
		sweep:   sweep,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	go s.run()

	return s, nil
}

// Allow consumes tokens for key and reports the decision.
//
// Costs above burst are capped at burst for parity with the Redis adapter.
// bucket.last is touched on every call (allowed and denied) so idle
// expiry measures time since last activity, not last success.
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return ratelimit.Decision{}, ratelimit.ErrClosed
	}

	b, ok := s.buckets[key]
	if !ok {
		b = &bucket{tokens: s.burst, last: now}
		s.buckets[key] = b
	} else if now.Sub(b.last) > s.idle {
		// Opportunistic expiry: treat idle bucket as new.
		b.tokens = s.burst
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	b.tokens = min(s.burst, b.tokens+elapsed*s.rate)
	b.last = now

	if b.tokens >= need {
		b.tokens -= need
		return ratelimit.Decision{Allowed: true, Remaining: b.tokens}, nil
	}

	retry := time.Duration((need - b.tokens) / s.rate * float64(time.Second))

	return ratelimit.Decision{Allowed: false, RetryAfter: retry, Remaining: b.tokens}, nil
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return ratelimit.ErrClosed
	}

	delete(s.buckets, key)

	return nil
}

// Close stops the background sweeper, wipes state, and is idempotent.
func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.stop)
	<-s.done

	s.mu.Lock()
	s.buckets = make(map[string]*bucket)
	s.mu.Unlock()

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

func (s *store) sweepOnce() {
	now := time.Now()

	s.mu.RLock()

	var expired []string

	for k, b := range s.buckets {
		if now.Sub(b.last) > s.idle {
			expired = append(expired, k)
		}
	}

	s.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, k := range expired {
		if b, ok := s.buckets[k]; ok && now.Sub(b.last) > s.idle {
			delete(s.buckets, k)
		}
	}
}
