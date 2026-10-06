package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/session"
)

// TestEdgeIssueVerify_concurrent proves the adapter is safe for concurrent
// issue and verify against a shared in-memory store.
func TestEdgeIssueVerify_concurrent(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			tok, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour)
			if err != nil {
				errs <- err

				return
			}

			if _, err := a.Verify(ctx, tok.Value); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Issue/Verify: %v", err)
	}
}

// stubStore is a session.Store test double whose Create/Save/Get/Delete
// results are scripted per test.
type stubStore struct {
	createFunc func(ctx context.Context, ttl time.Duration) (session.Session, error)
	saveFunc   func(ctx context.Context, sess session.Session) error
	getFunc    func(ctx context.Context, id string) (session.Session, error)
	deleteFunc func(ctx context.Context, id string) error
	closed     int
}

var _ session.Store = (*stubStore)(nil)

func (s *stubStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	if s.createFunc != nil {
		return s.createFunc(ctx, ttl)
	}
	return session.NewSession(session.NewID(), ttl), nil
}

func (s *stubStore) Save(ctx context.Context, sess session.Session) error {
	if s.saveFunc != nil {
		return s.saveFunc(ctx, sess)
	}
	return nil
}

func (s *stubStore) Get(ctx context.Context, id string) (session.Session, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, id)
	}
	return session.Session{}, session.ErrNotFound
}

func (s *stubStore) Delete(ctx context.Context, id string) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, id)
	}
	return nil
}

func (s *stubStore) Close() error {
	s.closed++
	return nil
}

// TestEdgeIssue_canceledContext proves Issue fails fast with the context
// error wrapped as "session: issue" when the caller's context is canceled.
func TestEdgeIssue_canceledContext(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, newMemoryStore(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := a.Issue(ctx, "u", nil, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Issue(canceled) err = %v, want context.Canceled", err)
	}
}

// TestEdgeVerify_canceledContext proves Verify surfaces the store's context
// error wrapped as "session: verify" rather than masking it as a miss.
func TestEdgeVerify_canceledContext(t *testing.T) {
	t.Parallel()

	st := &stubStore{
		getFunc: func(ctx context.Context, _ string) (session.Session, error) {
			return session.Session{}, ctx.Err()
		},
	}
	a := newAdapter(t, st)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := a.Verify(ctx, session.NewID())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify(canceled) err = %v, want context.Canceled", err)
	}
}

// TestEdgeRevoke_canceledContext proves Revoke surfaces the store's context
// error wrapped as "session: revoke".
func TestEdgeRevoke_canceledContext(t *testing.T) {
	t.Parallel()

	st := &stubStore{
		deleteFunc: func(ctx context.Context, _ string) error {
			return ctx.Err()
		},
	}
	a := newAdapter(t, st)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := a.Revoke(ctx, session.NewID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Revoke(canceled) err = %v, want context.Canceled", err)
	}
}

// TestEdgeVerify_storeErrorWrapped proves a non-ErrNotFound store failure is
// wrapped as "session: verify" and never reported as a generic invalid token.
func TestEdgeVerify_storeErrorWrapped(t *testing.T) {
	t.Parallel()

	boom := errors.New("db down")
	st := &stubStore{
		getFunc: func(_ context.Context, _ string) (session.Session, error) {
			return session.Session{}, boom
		},
	}
	a := newAdapter(t, st)

	_, err := a.Verify(t.Context(), session.NewID())
	if !errors.Is(err, boom) {
		t.Fatalf("Verify err = %v, want wrap store error", err)
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify err = %v, must not be ErrInvalidToken", err)
	}
}

// TestEdgeIssue_saveFailureCleansUpOrphan proves a Save failure during Issue
// deletes the created session best-effort and wraps the store error.
func TestEdgeIssue_saveFailureCleansUpOrphan(t *testing.T) {
	t.Parallel()

	boom := errors.New("save exploded")
	var deletedID string
	st := &stubStore{
		saveFunc: func(_ context.Context, _ session.Session) error {
			return boom
		},
		deleteFunc: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	a := newAdapter(t, st)

	_, err := a.Issue(t.Context(), "u", nil, time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("Issue err = %v, want wrap save error", err)
	}
	if deletedID == "" {
		t.Fatal("Issue did not best-effort delete the orphan session after Save failure")
	}
}

// TestEdgeClose_idempotent proves repeated Close calls keep succeeding; the
// adapter delegates to the store, which owns its own idempotency.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	st := &stubStore{}
	a := newAdapter(t, st)

	if err := a.Close(); err != nil {
		t.Fatalf("Close #1 err = %v, want nil", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close #2 err = %v, want nil", err)
	}
	if st.closed != 2 {
		t.Fatalf("store Close calls = %d, want 2 (adapter delegates each call)", st.closed)
	}
}

// TestEdgeIssue_createErrorWrapped proves a Create failure is wrapped as
// "session: issue" and never minted into a token.
func TestEdgeIssue_createErrorWrapped(t *testing.T) {
	t.Parallel()

	boom := errors.New("create exploded")
	st := &stubStore{
		createFunc: func(_ context.Context, _ time.Duration) (session.Session, error) {
			return session.Session{}, boom
		},
	}
	a := newAdapter(t, st)

	_, err := a.Issue(t.Context(), "u", nil, time.Minute)
	if !errors.Is(err, boom) {
		t.Fatalf("Issue err = %v, want wrap create error", err)
	}
}
