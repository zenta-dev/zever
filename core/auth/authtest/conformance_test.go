package authtest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/auth/authtest"
)

type stubRecord struct {
	subject string
	custom  map[string]any
	exp     time.Time
}

// stubAuth is a strict in-memory auth.Auth proving the kit passes when a
// backend honors every canonical sentinel distinctly.
type stubAuth struct {
	mu      sync.Mutex
	records map[string]stubRecord
	revoked map[string]bool
	next    int
}

func newStubAuth() *stubAuth {
	return &stubAuth{records: make(map[string]stubRecord), revoked: make(map[string]bool)}
}

func (s *stubAuth) Issue(_ context.Context, subject string, custom map[string]any, ttl time.Duration) (auth.Token, error) {
	if subject == "" {
		return auth.Token{}, fmt.Errorf("stub: issue: %w: empty subject", auth.ErrInvalidToken)
	}
	if ttl <= 0 {
		return auth.Token{}, fmt.Errorf("stub: issue: %w: non-positive ttl", auth.ErrInvalidToken)
	}
	if _, err := json.Marshal(custom); err != nil {
		return auth.Token{}, fmt.Errorf("stub: issue: signing failed: %w", errors.Join(auth.ErrInvalidToken, err))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	value := fmt.Sprintf("stub-token-%d", s.next)
	exp := time.Now().Add(ttl)
	cp := make(map[string]any, len(custom))
	for k, v := range custom {
		cp[k] = v
	}
	if len(cp) == 0 {
		cp = nil
	}
	s.records[value] = stubRecord{subject: subject, custom: cp, exp: exp}
	return auth.Token{Value: value, ExpiresAt: exp}, nil
}

func (s *stubAuth) Verify(_ context.Context, token string) (auth.Claims, error) {
	if token == "" {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked[token] {
		return auth.Claims{}, auth.ErrTokenRevoked
	}
	rec, ok := s.records[token]
	if !ok {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	if time.Now().After(rec.exp) {
		delete(s.records, token)
		return auth.Claims{}, auth.ErrTokenExpired
	}
	return auth.Claims{Subject: rec.subject, Custom: rec.custom, ExpiresAt: rec.exp}, nil
}

func (s *stubAuth) Revoke(_ context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("stub: revoke: %w: empty token", auth.ErrInvalidToken)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[token]; !ok && !s.revoked[token] {
		return nil
	}
	delete(s.records, token)
	s.revoked[token] = true
	return nil
}

func (s *stubAuth) Close() error { return nil }

// TestConformanceStub proves the kit passes against a strict in-memory backend.
func TestConformanceStub(t *testing.T) {
	t.Parallel()

	authtest.Conformance(t, func(t *testing.T) auth.Auth {
		t.Helper()
		return newStubAuth()
	})
}
