package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/session"
)

// TestContextCanceled verifies every operation fails fast with the context
// error when the caller's context is already canceled.
func TestContextCanceled(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Create(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create err = %v, want context.Canceled", err)
	}

	if _, err := s.Get(ctx, session.NewID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get err = %v, want context.Canceled", err)
	}

	if err := s.Save(ctx, session.Session{ID: session.NewID()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save err = %v, want context.Canceled", err)
	}

	if err := s.Delete(ctx, session.NewID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete err = %v, want context.Canceled", err)
	}
}

// TestAfterClose_errors verifies Get, Save, and Delete (not just Create) all
// report ErrClosed after Close, rather than panicking or touching the DB.
func TestAfterClose_errors(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	sess, err := s.Create(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := s.Get(ctx, sess.ID); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Get after Close = %v, want ErrClosed", err)
	}

	if err := s.Save(ctx, sess); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Save after Close = %v, want ErrClosed", err)
	}

	if err := s.Delete(ctx, sess.ID); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Delete after Close = %v, want ErrClosed", err)
	}
}

// TestConcurrentAccess exercises create/save/get/delete from many goroutines
// against one store, with the race detector enforcing lock correctness.
func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx := t.Context()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			sess, err := s.Create(ctx, time.Hour)
			if err != nil {
				t.Errorf("Create err = %v, want nil", err)
				return
			}

			sess.Data = map[string]any{"k": "v"}

			if err := s.Save(ctx, sess); err != nil {
				t.Errorf("Save err = %v, want nil", err)
				return
			}

			if _, err := s.Get(ctx, sess.ID); err != nil {
				t.Errorf("Get err = %v, want nil", err)
				return
			}

			if err := s.Delete(ctx, sess.ID); err != nil {
				t.Errorf("Delete err = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}

// mustNewDriver returns the white-box *driver behind a fresh store.
func mustNewDriver(t *testing.T) *driver {
	t.Helper()

	s := mustNew(t)

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("store type = %T, want *driver", s)
	}

	return d
}

// TestEdgeOpenFromDB_alias proves OpenFromDB behaves exactly like NewFromDB:
// it borrows the caller's connection and leaves it open on Close.
func TestEdgeOpenFromDB_alias(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/alias.db"

	conn, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	s, err := OpenFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("OpenFromDB failed: %v", err)
	}

	ctx := t.Context()

	sess, err := s.Create(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := s.Get(ctx, sess.ID); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("injected DB closed by OpenFromDB Close: %v", err)
	}
}

// TestEdgeNew_invalidOptions proves New rejects invalid Options before
// touching any database.
func TestEdgeNew_invalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{TTL: -time.Second}); err == nil {
		t.Fatal("New(negative TTL) = nil, want error")
	}
}

// TestEdgeNewFromDB_invalidOptions proves NewFromDB rejects invalid Options
// and never closes the caller's connection.
func TestEdgeNewFromDB_invalidOptions(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: t.TempDir() + "/keep.db"})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	if _, err := NewFromDB(conn, Options{TTL: -time.Second}); err == nil {
		t.Fatal("NewFromDB(negative TTL) = nil, want error")
	}
	if err := conn.Ping(t.Context()); err != nil {
		t.Fatalf("caller DB closed by failed NewFromDB: %v", err)
	}
}

// TestEdgeCreate_zeroTTLUsesStoreDefault proves Create with ttl <= 0 falls
// back to the store default TTL.
func TestEdgeCreate_zeroTTLUsesStoreDefault(t *testing.T) {
	t.Parallel()

	const ttl = 30 * time.Minute

	s, err := New(Options{TTL: ttl, Options: coredb.Options{Path: t.TempDir() + "/default.db"}})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	before := time.Now()

	sess, err := s.Create(t.Context(), 0)
	if err != nil {
		t.Fatalf("Create(0) failed: %v", err)
	}

	if sess.ExpiresAt.Sub(before) < ttl-time.Minute || sess.ExpiresAt.Sub(before) > ttl+time.Minute {
		t.Fatalf("Create(0) ExpiresAt = %v, want ~now+%v", sess.ExpiresAt, ttl)
	}
}

// TestEdgeGet_corruptRecord proves a record whose payload is not decodable
// fails closed with a wrapped decode error and the row is deleted so the
// corrupt record is never replayed.
func TestEdgeGet_corruptRecord(t *testing.T) {
	t.Parallel()

	s := mustNewDriver(t)
	ctx := t.Context()

	id := session.NewID()

	if err := s.kv.Set(ctx, s.key(id), []byte("{not-json"), time.Hour); err != nil {
		t.Fatalf("plant corrupt record: %v", err)
	}

	_, err := s.Get(ctx, id)
	if err == nil {
		t.Fatal("Get(corrupt) = nil, want decode error")
	}
	if errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get(corrupt) = %v, must not be ErrNotFound (fail closed)", err)
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Get(corrupt) err = %v, want decode error", err)
	}

	// Row deleted: a follow-up miss reports ErrNotFound, not the decode error.
	if _, err := s.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after corrupt err = %v, want ErrNotFound (row purged)", err)
	}
}

// TestEdgeGet_expiredRecord proves an expired record is reported as missing
// and purged best-effort, indistinguishable from a never-existing ID.
func TestEdgeGet_expiredRecord(t *testing.T) {
	t.Parallel()

	s := mustNewDriver(t)
	ctx := t.Context()

	id := session.NewID()
	past := time.Now().Add(-time.Hour)

	if err := s.kv.Set(ctx, s.key(id), mustEncode(t, wireSession{
		Data:      map[string]any{"k": "v"},
		CreatedAt: past.UnixNano(),
		UpdatedAt: past.UnixNano(),
		ExpiresAt: past.UnixNano(),
	}), time.Hour); err != nil {
		t.Fatalf("plant expired record: %v", err)
	}

	if _, err := s.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get(expired) err = %v, want ErrNotFound", err)
	}
}

// TestEdgeSave_overExpiredRecord proves Save against an expired record writes
// fresh under the store default TTL, ignoring any caller-provided ExpiresAt.
func TestEdgeSave_overExpiredRecord(t *testing.T) {
	t.Parallel()

	s := mustNewDriver(t)
	ctx := t.Context()

	id := session.NewID()
	past := time.Now().Add(-time.Hour)

	if err := s.kv.Set(ctx, s.key(id), mustEncode(t, wireSession{
		Data:      map[string]any{"old": true},
		CreatedAt: past.UnixNano(),
		UpdatedAt: past.UnixNano(),
		ExpiresAt: past.UnixNano(),
	}), time.Hour); err != nil {
		t.Fatalf("plant expired record: %v", err)
	}

	before := time.Now()

	if err := s.Save(ctx, session.Session{ID: id, Data: map[string]any{"k": "v"}}); err != nil {
		t.Fatalf("Save over expired failed: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after Save-over-expired failed: %v", err)
	}

	if _, ok := got.Data["old"]; ok {
		t.Fatal("Save over expired kept stale data")
	}
	if got.Data["k"] != "v" {
		t.Fatalf("Data = %v, want k=v", got.Data)
	}
	if got.ExpiresAt.Sub(before) < session.DefaultTTL-time.Minute {
		t.Fatalf("Save-over-expired ExpiresAt = %v, want ~now+%v (fresh default)", got.ExpiresAt, session.DefaultTTL)
	}
}

// TestEdgeSave_invalidID proves Save rejects malformed session IDs with a
// wrapped ErrInvalidID.
func TestEdgeSave_invalidID(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	for _, id := range []string{"", "short", "bad id"} {
		err := s.Save(t.Context(), session.Session{ID: id})
		if !errors.Is(err, session.ErrInvalidID) {
			t.Fatalf("Save(%q) err = %v, want ErrInvalidID", id, err)
		}
	}
}

// TestEdgeDelete_invalidID proves Delete rejects malformed session IDs.
func TestEdgeDelete_invalidID(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	for _, id := range []string{"", "short", "bad id"} {
		if err := s.Delete(t.Context(), id); !errors.Is(err, session.ErrInvalidID) {
			t.Fatalf("Delete(%q) err = %v, want ErrInvalidID", id, err)
		}
	}
}

// TestEdgeKVTTL proves the kv TTL resolution: zero expiry means no expiry,
// and remaining lifetimes at or below zero are floored against clock skew.
func TestEdgeKVTTL(t *testing.T) {
	t.Parallel()

	now := time.Now()

	if got := kvTTL(time.Time{}, now); got != 0 {
		t.Fatalf("kvTTL(zero) = %v, want 0", got)
	}
	if got := kvTTL(now.Add(-time.Hour), now); got != DefaultMinPreserveTTL {
		t.Fatalf("kvTTL(past) = %v, want %v", got, DefaultMinPreserveTTL)
	}
	if got := kvTTL(now, now); got != DefaultMinPreserveTTL {
		t.Fatalf("kvTTL(now) = %v, want %v", got, DefaultMinPreserveTTL)
	}

	remaining := 2 * time.Second
	if got := kvTTL(now.Add(remaining), now); got != remaining {
		t.Fatalf("kvTTL(future) = %v, want %v", got, remaining)
	}
}

// mustEncode encodes a wireSession the same infallible way the driver does.
func mustEncode(t *testing.T, w wireSession) []byte {
	t.Helper()

	buf, err := sessionCodec.Encode(w)
	if err != nil {
		t.Fatalf("encode wireSession: %v", err)
	}

	return buf
}
