package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/session"
)

// mustNew opens a fresh file-backed store per test: ":memory:" sqlite uses
// shared cache (process-global), so fixed IDs would collide across reruns
// and leak between parallel tests.
func mustNew(t *testing.T) session.Store {
	t.Helper()

	s, err := New(Options{Options: coredb.Options{Path: filepath.Join(t.TempDir(), "session.db")}})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestCreateGetRoundTrip(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	sess, err := s.Create(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := s.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != sess.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, sess.ID)
	}
}

func TestGetMiss(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}

	if _, err := s.Get(ctx, "bad-id"); err == nil {
		t.Fatal("Get bad id = nil, want error")
	}
}

func TestSavePreservesExpiry(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	sess, err := s.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	want := sess.ExpiresAt

	sess.Data = map[string]any{"k": "v"}

	if serr := s.Save(ctx, sess); serr != nil {
		t.Fatalf("Save failed: %v", serr)
	}

	got, err := s.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !got.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v (preserved)", got.ExpiresAt, want)
	}

	if got.Data["k"] != "v" {
		t.Fatalf("Data = %v, want k=v", got.Data)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	sess, err := s.Create(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := s.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := s.Get(ctx, sess.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}

	if err := s.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete missing failed: %v", err)
	}
}

func TestNewFromDBBorrowsConnection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared.db")

	conn, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	s, err := NewFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB failed: %v", err)
	}

	ctx := t.Context()

	sess, err := s.Create(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := s.Get(ctx, sess.ID); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Borrowed connection: store Close must not close the injected DB.
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("injected DB closed by store Close: %v", err)
	}

	if _, err := NewFromDB(nil, Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	if _, err := s.Create(t.Context(), time.Minute); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Create after Close = %v, want ErrClosed", err)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	if err := (Options{TTL: -time.Second}).Validate(); err == nil {
		t.Error("Validate negative TTL = nil, want error")
	}

	if err := (Options{}).Validate(); err != nil {
		t.Errorf("Validate empty = %v, want nil", err)
	}
}

func TestRegisterOpensViaCoreOptions(t *testing.T) {
	Register()

	s, err := session.Open(Adapter, session.Options{})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
}
