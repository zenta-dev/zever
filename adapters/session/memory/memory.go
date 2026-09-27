package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/session"
)

var _ session.Store = (*store)(nil)

// DefaultMinSweepInterval floors the derived background sweep period.
const DefaultMinSweepInterval = time.Second

// DefaultMaxSweepInterval caps the derived background sweep period.
const DefaultMaxSweepInterval = 5 * time.Minute

type record struct {
	sess session.Session
}

type store struct {
	mu     sync.RWMutex
	m      map[string]*record
	ttl    time.Duration
	now    func() time.Time
	closed atomic.Bool
	stop   chan struct{}
	wg     sync.WaitGroup
}

// New creates an in-memory session.Store with the given default TTL.
// Zero TTL means session.DefaultTTL. Negative TTL fails validation.
func New(opts session.Options) (session.Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = session.DefaultTTL
	}
	s := &store{
		m:    make(map[string]*record),
		ttl:  ttl,
		now:  time.Now,
		stop: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.sweepLoop(sweepInterval(ttl))
	return s, nil
}

// sweepInterval derives the background sweep period from the store TTL:
// ttl/2 clamped to [DefaultMinSweepInterval, DefaultMaxSweepInterval].
func sweepInterval(ttl time.Duration) time.Duration {
	iv := ttl / 2
	if iv < DefaultMinSweepInterval {
		return DefaultMinSweepInterval
	}
	if iv > DefaultMaxSweepInterval {
		return DefaultMaxSweepInterval
	}
	return iv
}

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
	s.mu.Lock()
	s.sweepLocked(s.now())
	s.m[sess.ID] = &record{sess: sess.Clone()}
	s.mu.Unlock()
	return sess, nil
}

func (s *store) Get(ctx context.Context, id string) (session.Session, error) {
	if err := ctx.Err(); err != nil {
		return session.Session{}, err
	}
	if err := session.ValidateID(id); err != nil {
		return session.Session{}, fmt.Errorf("memory: %w", err)
	}
	if s.closed.Load() {
		return session.Session{}, session.ErrClosed
	}
	now := s.now()
	s.mu.RLock()
	rec, ok := s.m[id]
	if !ok {
		s.mu.RUnlock()
		return session.Session{}, session.ErrNotFound
	}
	cp := rec.sess.Clone()
	s.mu.RUnlock()
	if cp.Expired(now) {
		s.mu.Lock()
		if cur, ok := s.m[id]; ok {
			if cur.sess.Expired(s.now()) {
				delete(s.m, id)
			}
		}
		s.mu.Unlock()
		return session.Session{}, session.ErrNotFound
	}
	return cp, nil
}

func (s *store) Save(ctx context.Context, sess session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := session.ValidateID(sess.ID); err != nil {
		return fmt.Errorf("memory: %w", err)
	}
	if s.closed.Load() {
		return session.ErrClosed
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	if rec, ok := s.m[sess.ID]; ok {
		// Live entry: sweepLocked above purged expired records under
		// this same lock, so rec cannot be expired here.
		// Last-write-wins on Data, touch UpdatedAt, keep original
		// absolute ExpiresAt.
		cp := sess.Clone()
		cp.ExpiresAt = rec.sess.ExpiresAt
		cp.CreatedAt = rec.sess.CreatedAt
		cp.UpdatedAt = now
		rec.sess = cp
		return nil
	}
	cp := sess.Clone()
	cp.UpdatedAt = now
	// Missing (or previously expired) ID: fail-closed fresh expiry,
	// ignoring any caller-provided ExpiresAt.
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.ExpiresAt = now.Add(s.ttl)
	s.m[sess.ID] = &record{sess: cp}
	return nil
}

func (s *store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := session.ValidateID(id); err != nil {
		return fmt.Errorf("memory: %w", err)
	}
	if s.closed.Load() {
		return session.ErrClosed
	}
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
	return nil
}

func (s *store) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.stop)
	s.wg.Wait()
	s.mu.Lock()
	s.m = make(map[string]*record)
	s.mu.Unlock()
	return nil
}

// sweepLocked deletes expired entries; caller must hold s.mu (write).
func (s *store) sweepLocked(now time.Time) {
	for id, rec := range s.m {
		if rec.sess.Expired(now) {
			delete(s.m, id)
		}
	}
}

func (s *store) sweepLoop(interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			s.sweepLocked(now)
			s.mu.Unlock()
		}
	}
}
