package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
)

// mustNew opens a fresh file-backed cache per test: ":memory:" sqlite uses
// shared cache (process-global), so fixed keys would collide across reruns
// and leak between parallel tests.
func mustNew(tb testing.TB) cache.Cache {
	tb.Helper()

	c, err := New(Options{Options: coredb.Options{Path: filepath.Join(tb.TempDir(), "cache.db")}})
	if err != nil {
		tb.Fatalf("New failed: %v", err)
	}

	tb.Cleanup(func() { _ = c.Close(tb.Context()) })

	return c
}

func TestSetGetRoundTrip(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got) != "v" {
		t.Fatalf("Get = %q, want v", got)
	}
}

func TestGetMiss(t *testing.T) {
	t.Parallel()

	c := mustNew(t)

	if _, err := c.Get(t.Context(), "missing"); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestIncrementNonInteger(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	if err := c.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	err := c.Increment(ctx, "bad")
	if !errors.Is(err, cache.ErrInvalidValue) {
		t.Fatalf("Increment(abc) = %v, want ErrInvalidValue", err)
	}

	var invErr *cache.InvalidValueError
	if !errors.As(err, &invErr) {
		t.Fatalf("errors.As(err, InvalidValueError) = false (err = %T %v)", err, err)
	}

	if invErr.Key != "bad" {
		t.Fatalf("InvalidValueError.Key = %q, want bad", invErr.Key)
	}
}

func TestPrefixesIsolateKeys(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "shared.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	ctx := t.Context()
	t.Cleanup(func() { _ = conn.Close(ctx) })

	a, err := NewFromDB(conn, Options{Prefix: "a"})
	if err != nil {
		t.Fatalf("NewFromDB(a) failed: %v", err)
	}

	t.Cleanup(func() { _ = a.Close(ctx) })

	b, err := NewFromDB(conn, Options{Prefix: "b"})
	if err != nil {
		t.Fatalf("NewFromDB(b) failed: %v", err)
	}

	t.Cleanup(func() { _ = b.Close(ctx) })

	if err := a.Set(ctx, "k", []byte("va"), time.Minute); err != nil {
		t.Fatalf("Set(a) failed: %v", err)
	}

	if _, err := b.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get(b) = %v, want ErrNotFound (prefix isolation)", err)
	}
}

func TestNewFromDBBorrowsConnection(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "shared.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	ctx := t.Context()
	t.Cleanup(func() { _ = conn.Close(ctx) })

	c, err := NewFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB failed: %v", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Borrowed connection: cache Close must not close the injected DB.
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("injected DB closed by cache Close: %v", err)
	}

	if _, err := NewFromDB(nil, Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := c.Close(ctx); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Fatalf("Get after Close = %v, want ErrClosed", err)
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

	c, err := cache.Open(Adapter, cache.Options{})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("Close = %v", err)
	}
}
