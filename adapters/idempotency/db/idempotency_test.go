package db

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/idempotency"
)

// mustNew opens a fresh file-backed store per test: ":memory:" sqlite uses
// shared cache (process-global), so fixed keys would collide across reruns
// and leak between parallel tests.
func mustNew(t *testing.T) idempotency.Store {
	t.Helper()

	s, err := New(Options{Options: coredb.Options{Path: filepath.Join(t.TempDir(), "idem.db")}})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestBeginCompleteReplay(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()
	fp := []byte("fp1")

	out, err := s.Begin(ctx, "k1", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if out.Replay {
		t.Fatal("first Begin Replay = true, want false")
	}

	_, err = s.Begin(ctx, "k1", idempotency.BeginOptions{Fingerprint: fp})
	if !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("second Begin = %v, want ErrInProgress", err)
	}

	if err = s.Complete(ctx, "k1", fp, []byte("res")); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	out, err = s.Begin(ctx, "k1", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin after Complete failed: %v", err)
	}

	if !out.Replay || string(out.Result) != "res" {
		t.Fatalf("Begin replay = (%v, %q), want (true, res)", out.Replay, out.Result)
	}
}

func TestFingerprintMismatch(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Mismatch reports even mid-flight (pending record).
	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin mismatch = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "k", []byte("b"), []byte("res")); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Complete mismatch = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "k", []byte("a"), []byte("res")); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin done mismatch = %v, want ErrKeyMismatch", err)
	}
}

func TestCompleteWithoutBeginUpserts(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()
	fp := []byte("fp")

	if err := s.Complete(ctx, "k", fp, []byte("late")); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	out, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if !out.Replay || string(out.Result) != "late" {
		t.Fatalf("Begin replay = (%v, %q), want (true, late)", out.Replay, out.Result)
	}
}

func TestForget(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()
	fp := []byte("fp")

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if err := s.Forget(ctx, "k"); err != nil {
		t.Fatalf("Forget failed: %v", err)
	}

	// Missing keys are idempotent.
	if err := s.Forget(ctx, "k"); err != nil {
		t.Fatalf("Forget missing failed: %v", err)
	}

	out, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin after Forget failed: %v", err)
	}

	if out.Replay {
		t.Fatal("Begin after Forget Replay = true, want false")
	}
}

func TestFingerprintTooLarge(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()
	big := []byte(strings.Repeat("f", maxFingerprintLen+1))

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: big}); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Begin big fp = %v, want ErrFingerprintTooLarge", err)
	}

	if err := s.Complete(ctx, "k", big, nil); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Complete big fp = %v, want ErrFingerprintTooLarge", err)
	}
}

func TestInvalidKey(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	if _, err := s.Begin(ctx, "", idempotency.BeginOptions{}); err == nil {
		t.Error("Begin empty key = nil, want error")
	}

	if err := s.Complete(ctx, "", nil, nil); err == nil {
		t.Error("Complete empty key = nil, want error")
	}

	if err := s.Forget(ctx, ""); err == nil {
		t.Error("Forget empty key = nil, want error")
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
	fp := []byte("fp")

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if err := s.Complete(ctx, "k", fp, []byte("r")); err != nil {
		t.Fatalf("Complete failed: %v", err)
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

	if _, err := s.Begin(t.Context(), "k", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Begin after Close = %v, want ErrClosed", err)
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

	s, err := idempotency.Open(Adapter, idempotency.Options{})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
}
