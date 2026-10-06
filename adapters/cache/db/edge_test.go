package db

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
)

// TestEdgeSet_nonPositiveTTLPersists covers the TTL boundary: a zero or
// negative per-call ttl means persist, so the entry survives a Get.
func TestEdgeSet_nonPositiveTTLPersists(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	for _, ttl := range []time.Duration{0, -time.Second} {
		if err := c.Set(ctx, "k", []byte("v"), ttl); err != nil {
			t.Fatalf("Set(ttl=%v) = %v, want nil", ttl, err)
		}

		got, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get after Set(ttl=%v) = %v, want nil", ttl, err)
		}

		if string(got) != "v" {
			t.Fatalf("Get after Set(ttl=%v) = %q, want v", ttl, got)
		}
	}
}

// TestEdgeConcurrent_setGet exercises parallel Set/Get on distinct keys,
// proving the shared database pool is safe under concurrency.
func TestEdgeConcurrent_setGet(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	var (
		wg   sync.WaitGroup
		n    atomic.Int64
		errs = make(chan error, 16)
	)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 4 {
				key := "k-" + strconv.FormatInt(n.Add(1), 10)

				if err := c.Set(ctx, key, []byte("v"), 0); err != nil {
					errs <- err

					return
				}

				if _, err := c.Get(ctx, key); err != nil {
					errs <- err

					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Set/Get: %v", err)
	}
}

// TestEdgeOpenFromDB_roundTrip covers the OpenFromDB alias over a
// caller-owned connection.
func TestEdgeOpenFromDB_roundTrip(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "cache.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	ctx := t.Context()
	t.Cleanup(func() { _ = conn.Close(ctx) })

	c, err := OpenFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("OpenFromDB failed: %v", err)
	}

	t.Cleanup(func() { _ = c.Close(ctx) })

	if setErr := c.Set(ctx, "k", []byte("v"), time.Minute); setErr != nil {
		t.Fatalf("Set failed: %v", setErr)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got) != "v" {
		t.Fatalf("Get = %q, want v", got)
	}
}

// TestEdge_canceledContext_allOps covers the ctx-first guard on every
// exported op: a canceled context surfaces context.Canceled before any IO.
func TestEdge_canceledContext_allOps(t *testing.T) {
	t.Parallel()

	c := mustNew(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := c.Get(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get = %v, want context.Canceled", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("Set = %v, want context.Canceled", err)
	}

	if _, err := c.SetIfAbsent(ctx, "k", []byte("v"), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("SetIfAbsent = %v, want context.Canceled", err)
	}

	if err := c.Delete(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Delete = %v, want context.Canceled", err)
	}

	if err := c.Increment(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Increment = %v, want context.Canceled", err)
	}

	if err := c.Decrement(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Decrement = %v, want context.Canceled", err)
	}

	if _, err := c.Exists(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Exists = %v, want context.Canceled", err)
	}
}

// TestEdge_closed_allOps covers the closed guard on every exported op after
// Close.
func TestEdge_closed_allOps(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close = %v", err)
	}

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Get = %v, want ErrClosed", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Set = %v, want ErrClosed", err)
	}

	if _, err := c.SetIfAbsent(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("SetIfAbsent = %v, want ErrClosed", err)
	}

	if err := c.Delete(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Delete = %v, want ErrClosed", err)
	}

	if err := c.Increment(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Increment = %v, want ErrClosed", err)
	}

	if err := c.Decrement(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Decrement = %v, want ErrClosed", err)
	}

	if _, err := c.Exists(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Exists = %v, want ErrClosed", err)
	}
}

// TestEdgeNew_invalidDSN covers the postgres open failure path: a malformed
// DSN fails construction before any connection is owned.
func TestEdgeNew_invalidDSN(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: coredb.Options{DSN: "postgres://u:p@h:notaport/db"}})
	if err == nil {
		t.Fatal("New(invalid DSN) = nil, want error")
	}
}

// TestEdgeNew_negativeTTL covers the options-validation path: a negative TTL
// is rejected before any backend is opened.
func TestEdgeNew_negativeTTL(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{TTL: -time.Second}); err == nil {
		t.Fatal("New(negative TTL) = nil, want error")
	}
}

// TestEdgeRegisterSharedOpensViaCore proves Register wires the shared factory
// so cache.OpenShared resolves the adapter over a borrowed pool.
func TestEdgeRegisterSharedOpensViaCore(t *testing.T) {
	Register()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "cache.db")})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	ctx := t.Context()
	t.Cleanup(func() { _ = conn.Close(ctx) })

	c, err := cache.OpenShared(Adapter, conn, cache.Options{})
	if err != nil {
		t.Fatalf("OpenShared = %v", err)
	}

	if setErr := c.Set(ctx, "k", []byte("v"), time.Minute); setErr != nil {
		t.Fatalf("Set = %v", setErr)
	}

	// Borrowed pool: closing the cache must leave conn usable.
	if closeErr := c.Close(ctx); closeErr != nil {
		t.Fatalf("Close = %v", closeErr)
	}

	if pingErr := conn.Ping(ctx); pingErr != nil {
		t.Fatalf("borrowed pool closed by cache Close: %v", pingErr)
	}
}
