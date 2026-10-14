package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

// failDeleteStore fails Delete while delegating everything else.
type failDeleteStore struct {
	Store
	err error
}

func (s failDeleteStore) Delete(context.Context, string) error { return s.err }

// failSaveStore fails Save while delegating everything else.
type failSaveStore struct {
	Store
	err error
}

func (s failSaveStore) Save(context.Context, Session) error { return s.err }

// mustSeed creates a session carrying key/value data for rotation tests.
func mustSeed(t *testing.T, s *stubStore) Session {
	t.Helper()
	ctx := t.Context()
	old, err := s.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	old.Data["user"] = "alice"
	if err := s.Save(ctx, old); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return old
}

// TestRotate_CarriesDataForward pins the fixation fix: after rotation the
// data lives under a fresh ID and the pre-login ID is dead.
func TestRotate_CarriesDataForward(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	ctx := t.Context()
	old := mustSeed(t, store)

	fresh, err := Rotate(ctx, store, old.ID, time.Hour)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if fresh.ID == old.ID {
		t.Error("Rotate kept the old ID, want a fresh one")
	}
	if fresh.Data["user"] != "alice" {
		t.Errorf("Rotate data = %v, want user carried forward", fresh.Data)
	}
	if _, err := store.Get(ctx, old.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(old) err = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(ctx, fresh.ID); err != nil {
		t.Errorf("Get(fresh) err = %v, want nil", err)
	}
}

// TestRotate_MissingOrExpiredOld_FailsClosed ensures rotation never mints a
// session when the old one cannot be resolved, and leaks no replacement.
func TestRotate_MissingOrExpiredOld_FailsClosed(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	ctx := t.Context()

	if _, err := Rotate(ctx, store, NewID(), time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("Rotate(missing) err = %v, want ErrNotFound", err)
	}

	expired, err := store.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if err := store.Save(ctx, expired); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := Rotate(ctx, store, expired.ID, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("Rotate(expired) err = %v, want ErrNotFound", err)
	}

	if n := len(store.sessions); n != 0 {
		t.Errorf("store holds %d sessions, want 0 (no replacement leaked)", n)
	}
}

// TestRotate_InvalidID_FailsClosed ensures malformed IDs fail without
// touching the store and never echo ID bytes.
func TestRotate_InvalidID_FailsClosed(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	if _, err := Rotate(t.Context(), store, "bogus", time.Hour); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Rotate(bogus) err = %v, want ErrInvalidID", err)
	}
	if n := len(store.sessions); n != 0 {
		t.Errorf("store holds %d sessions, want 0", n)
	}
}

// TestRotate_NilStore_Fails ensures a nil store is an error, not a panic.
func TestRotate_NilStore_Fails(t *testing.T) {
	t.Parallel()

	if _, err := Rotate(t.Context(), nil, NewID(), time.Hour); err == nil {
		t.Fatal("Rotate(nil store) = nil, want error")
	}
}

// TestRotate_DeleteOldFailure_ReturnsNew proves the fail-open-on-cleanup
// contract: when the old ID cannot be deleted, the caller still gets a
// usable fresh session plus the cleanup error.
func TestRotate_DeleteOldFailure_ReturnsNew(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	ctx := t.Context()
	old := mustSeed(t, store)

	boom := errors.New("delete boom")
	fresh, err := Rotate(ctx, failDeleteStore{Store: store, err: boom}, old.ID, time.Hour)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Rotate err = %v, want delete boom", err)
	}
	if _, getErr := store.Get(ctx, fresh.ID); getErr != nil {
		t.Errorf("Get(fresh) err = %v, want nil (fresh session is usable)", getErr)
	}
}

// TestRotate_SaveFailure_CleansUp ensures a failed replacement write does
// not leave an orphan fresh session behind.
func TestRotate_SaveFailure_CleansUp(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	ctx := t.Context()
	old := mustSeed(t, store)

	if _, err := Rotate(ctx, failSaveStore{Store: store, err: errors.New("save boom")}, old.ID, time.Hour); err == nil {
		t.Fatal("Rotate(save failure) = nil, want error")
	}
	if n := len(store.sessions); n != 1 {
		t.Errorf("store holds %d sessions, want 1 (old kept, fresh cleaned)", n)
	}
	if _, err := store.Get(ctx, old.ID); err != nil {
		t.Errorf("Get(old) err = %v, want nil (old kept on save failure)", err)
	}
}

// TestRotate_CanceledContext_Fails ensures context cancellation aborts
// rotation deterministically.
func TestRotate_CanceledContext_Fails(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	old := mustSeed(t, store)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Rotate(ctx, store, old.ID, time.Hour); err == nil {
		t.Fatal("Rotate(canceled) = nil, want error")
	}
}
