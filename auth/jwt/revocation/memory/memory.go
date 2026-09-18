// Package memory provides an in-process revocation.Store. It has no
// cross-instance or durability story: revocations are lost on restart and
// are not visible to other processes. Use auth/jwt/revocation/redis when
// that matters.
package memory

import (
	"container/heap"
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

// expiryItem is one revoked jti tracked in the byExpiry min-heap, alongside
// its slice position (maintained by expiryHeap.Swap for container/heap).
type expiryItem struct {
	jti    string
	expiry time.Time
	index  int
}

// expiryHeap is a container/heap min-heap ordered by expiry time, so the
// soonest-to-expire entry always sits at index 0. It backs both:
//   - eviction at capacity: evict the entry closest to expiry first, instead
//     of the oldest-inserted one, so a mass-revocation burst can't push out
//     a revocation that still has most of its life left.
//   - pruning: since the root is always the soonest-to-expire *live* entry,
//     popping while the root is expired visits exactly the expired entries
//     and stops as soon as it finds one that hasn't expired yet.
type expiryHeap []*expiryItem

func (h expiryHeap) Len() int { return len(h) }

func (h expiryHeap) Less(i, j int) bool { return h[i].expiry.Before(h[j].expiry) }

func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *expiryHeap) Push(x any) {
	item, _ := x.(*expiryItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]

	return item
}

type store struct {
	max int

	mu       sync.Mutex
	revoked  map[string]time.Time
	byExpiry expiryHeap

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
	heap.Push(&s.byExpiry, &expiryItem{jti: jti, expiry: until})

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
	s.byExpiry = nil
	s.mu.Unlock()

	return nil
}

func (s *store) ensureCapacityLocked(now time.Time) {
	if len(s.revoked) < s.max {
		return
	}

	s.pruneExpiredLocked(now)
	if len(s.revoked) >= s.max {
		s.evictSoonestToExpireLocked()
	}
}

// pruneExpiredLocked drops every already-expired entry. Because byExpiry is
// a min-heap keyed by expiry, its root is always the soonest-to-expire live
// entry: once the root isn't expired, nothing behind it is either, so the
// loop can stop at the first live entry instead of scanning the whole map.
func (s *store) pruneExpiredLocked(now time.Time) {
	for len(s.byExpiry) > 0 && !now.Before(s.byExpiry[0].expiry) {
		item, _ := heap.Pop(&s.byExpiry).(*expiryItem)
		delete(s.revoked, item.jti)
	}
}

// evictSoonestToExpireLocked evicts the entry closest to its natural expiry,
// rather than the oldest-inserted one. Under a mass-revocation burst this
// minimizes the window in which an evicted-but-still-revoked jti becomes
// silently valid again: whatever gets evicted would have expired soonest
// anyway.
func (s *store) evictSoonestToExpireLocked() {
	if len(s.byExpiry) == 0 {
		return
	}

	item, _ := heap.Pop(&s.byExpiry).(*expiryItem)
	delete(s.revoked, item.jti)
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
