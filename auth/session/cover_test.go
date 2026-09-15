package session_test

// Coverage note: the memory.New error branch in New was pruned by evidence:
// sessionmemory.New with zero options performs no I/O and validates trivially,
// so it cannot fail. The error return is discarded via store, _ = assignment.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth"
	authsession "github.com/zenta-dev/zever/auth/session"
	"github.com/zenta-dev/zever/session"
)

// failStore injects infrastructure failures into every Store method.
type failStore struct {
	err error
}

func (f *failStore) Create(context.Context, time.Duration) (session.Session, error) {
	return session.Session{}, f.err
}

func (f *failStore) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, f.err
}

func (f *failStore) Save(context.Context, session.Session) error { return f.err }

func (f *failStore) Delete(context.Context, string) error { return f.err }

func (f *failStore) Close() error { return nil }

// saveFailStore creates fine but fails saves.
type saveFailStore struct {
	inner session.Store
}

func (s *saveFailStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return s.inner.Create(ctx, ttl)
}

func (s *saveFailStore) Get(ctx context.Context, id string) (session.Session, error) {
	return s.inner.Get(ctx, id)
}

func (s *saveFailStore) Save(context.Context, session.Session) error {
	return errors.New("boom")
}

func (s *saveFailStore) Delete(ctx context.Context, id string) error {
	return s.inner.Delete(ctx, id)
}

func (s *saveFailStore) Close() error { return nil }

func TestCoverNewInvalidOpts(t *testing.T) {
	t.Parallel()

	_, err := authsession.New(auth.Options{JWT: auth.JWTOptions{MaxTTL: -time.Second}})
	if err == nil {
		t.Fatal("New(invalid core opts) = nil, want validation error")
	}
}

func TestCoverIssueStoreCreateFails(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, &failStore{err: errors.New("boom")})
	_, err := a.Issue(context.Background(), "sub", nil, time.Minute)
	if err == nil {
		t.Fatal("Issue(store fail) = nil, want wrapped error")
	}
}

func TestCoverIssueStoreSaveFails(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, &saveFailStore{inner: newMemoryStore(t)})
	_, err := a.Issue(context.Background(), "sub", nil, time.Minute)
	if err == nil {
		t.Fatal("Issue(save fail) = nil, want wrapped error")
	}
}

func TestCoverVerifyStoreGetFails(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, &failStore{err: errors.New("boom")})
	_, err := a.Verify(context.Background(), session.NewID())
	if err == nil {
		t.Fatal("Verify(store fail) = nil, want wrapped error")
	}
	if errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Verify(store fail) = %v, must not map to ErrNotFound", err)
	}
}

func TestCoverRevokeStoreDeleteFails(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, &failStore{err: errors.New("boom")})
	if err := a.Revoke(context.Background(), session.NewID()); err == nil {
		t.Fatal("Revoke(store fail) = nil, want wrapped error")
	}
}

func plantEnvelope(t *testing.T, st session.Store, id string, data map[string]any) {
	t.Helper()

	if err := st.Save(context.Background(), session.Session{ID: id, Data: data}); err != nil {
		t.Fatalf("plant: %v", err)
	}
}

func TestCoverVerifyExpInt(t *testing.T) {
	t.Parallel()

	st := newMemoryStore(t)
	a := newAdapter(t, st)
	id := session.NewID()
	future := time.Now().Add(time.Hour).Unix()
	plantEnvelope(t, st, id, map[string]any{"sub": "u", "exp": int(future)})

	got, err := a.Verify(context.Background(), id)
	if err != nil {
		t.Fatalf("Verify(int exp) = %v, want nil", err)
	}
	if got.Subject != "u" {
		t.Errorf("Subject = %q, want u", got.Subject)
	}
}

func TestCoverVerifyExpFloat(t *testing.T) {
	t.Parallel()

	st := newMemoryStore(t)
	a := newAdapter(t, st)
	id := session.NewID()
	plantEnvelope(t, st, id, map[string]any{"sub": "u", "exp": float64(time.Now().Add(time.Hour).Unix())})

	if _, err := a.Verify(context.Background(), id); err != nil {
		t.Fatalf("Verify(float exp) = %v, want nil", err)
	}
}

func TestCoverVerifyExpZeroFails(t *testing.T) {
	t.Parallel()

	st := newMemoryStore(t)
	a := newAdapter(t, st)
	id := session.NewID()
	plantEnvelope(t, st, id, map[string]any{"sub": "u", "exp": int64(0)})

	if _, err := a.Verify(context.Background(), id); err == nil {
		t.Fatal("Verify(zero exp) = nil, want corrupt-envelope error")
	}
}
