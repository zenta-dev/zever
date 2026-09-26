package session

import (
	"context"
	"sync"
	"time"
)

// stubStore is a minimal in-memory Store honoring the contract.
// It exists so core behavior (expiry, miss, idempotent delete) is pinned
// without depending on the memory adapter owned by another agent.
type stubStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func newStubStore() *stubStore {
	return &stubStore{sessions: make(map[string]Session)}
}

func (s *stubStore) Create(ctx context.Context, ttl time.Duration) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	sess := NewSession(NewID(), ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess.Clone()
	return sess, nil
}

func (s *stubStore) Get(ctx context.Context, id string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	if err := ValidateID(id); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	if sess.Expired(time.Now()) {
		delete(s.sessions, id)
		return Session{}, ErrNotFound
	}
	return sess.Clone(), nil
}

func (s *stubStore) Save(ctx context.Context, sess Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateID(sess.ID); err != nil {
		return err
	}
	sess.UpdatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess.Clone()
	return nil
}

func (s *stubStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *stubStore) Close() error { return nil }
