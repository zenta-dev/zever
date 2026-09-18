// Package memory provides an in-process revocation.Store. It has no
// cross-instance or durability story: revocations are lost on restart and
// are not visible to other processes. Use auth/jwt/revocation/redis when
// that matters.
package memory

import (
	"context"
	"errors"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/auth/jwt/revocation"
)

// DefaultMaxEntries caps revocation-map memory (~5 MB at ~500 B/entry).
const DefaultMaxEntries = 10000

// newTicker constructs the pruner ticker, swappable in tests.
var newTicker = time.NewTicker

// Options configures the in-memory revocation.Store.
type Options struct {
	// MaxEntries caps the number of tracked revocations. <= 0 uses
	// DefaultMaxEntries.
	MaxEntries int
}

var _ revocation.Store = (*store)(nil)

type store struct {
	max int

	mu      sync.Mutex
	revoked map[string]time.Time
	order   []string
	head    int

	closed atomic.Bool
	stop   chan struct{}
	wg     sync.WaitGroup
}

// New creates an in-memory revocation.Store bounded at opts.MaxEntries
// entries. A background pruner sweeps expired entries roughly once a minute
// (jittered) so memory is bounded even without callers driving IsRevoked.
func New(opts Options) (revocation.Store, error) {
	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}

	s := &store{
		max:     maxEntries,
		revoked: make(map[string]time.Time),
		stop:    make(chan struct{}),
	}
	s.startPruner()

	return s, nil
}

// Revoke marks jti as revoked until until. It is idempotent.
func (s *store) Revoke(_ context.Context, jti string, until time.Time) error {
	if jti == "" {
		return errors.New("revocation/memory: revoke: empty jti")
	}
	if s.closed.Load() {
		return revocation.ErrClosed
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, dup := s.revoked[jti]; dup {
		return nil
	}

	s.ensureCapacityLocked(now)
	s.revoked[jti] = until
	s.order = append(s.order, jti)

	return nil
}

// IsRevoked reports whether jti is currently revoked.
func (s *store) IsRevoked(_ context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	if s.closed.Load() {
		return false, revocation.ErrClosed
	}

	s.mu.Lock()
	until, ok := s.revoked[jti]
	s.mu.Unlock()

	if !ok {
		return false, nil
	}

	return time.Now().Before(until), nil
}

// Close stops the background pruner and releases resources. It is
// idempotent.
func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.stop)
	s.wg.Wait()

	s.mu.Lock()
	clear(s.revoked)
	s.mu.Unlock()

	return nil
}

func (s *store) ensureCapacityLocked(now time.Time) {
	if len(s.revoked) < s.max {
		return
	}

	s.pruneExpiredLocked(now)
	if len(s.revoked) >= s.max {
		s.evictOldestLocked()
	}
}

func (s *store) pruneExpiredLocked(now time.Time) {
	for jti, until := range s.revoked {
		if !now.Before(until) {
			delete(s.revoked, jti)
		}
	}
}

func (s *store) evictOldestLocked() {
	for s.head < len(s.order) {
		jti := s.order[s.head]
		s.head++

		if _, ok := s.revoked[jti]; ok {
			delete(s.revoked, jti)
			break
		}
	}

	if s.head > 1024 && s.head > len(s.order)/2 {
		s.order = append([]string(nil), s.order[s.head:]...)
		s.head = 0
	}
}

func (s *store) startPruner() {
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer func() { _ = recover() }()

		// Jitter the 1m interval to avoid thundering herd when many stores
		// start together (e.g. rolling deploy).
		//nolint:gosec // math/rand suffices for non-security jitter.
		jitter := time.Duration(mrand.Int63n(int64(10*time.Second))) - 5*time.Second
		ticker := newTicker(time.Minute + jitter)
		defer ticker.Stop()

		for {
			select {
			case <-s.stop:
				return
			case now := <-ticker.C:
				s.pruneOnce(now)
			}
		}
	}()
}

// pruneOnce drops expired revocations under lock. Split from the loop for
// direct unit testing.
func (s *store) pruneOnce(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
}
