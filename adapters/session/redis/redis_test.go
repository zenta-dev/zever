package redis_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	sredis "github.com/zenta-dev/zever/adapters/session/redis"
	"github.com/zenta-dev/zever/core/session"
)

// keySeq keeps generated key prefixes unique even within a single test's
// shared miniredis instance.
var keySeq atomic.Int64

// testServer starts a per-test miniredis instance, auto-closed via
// t.Cleanup.
func testServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	return miniredis.RunT(t)
}

// testPrefix returns a unique, validation-safe key prefix per test.
func testPrefix(t *testing.T) string {
	t.Helper()

	const punct = "!#$%&'*+-.^_`|~"
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			return r
		case strings.ContainsRune(punct, r):
			return r
		default:
			return '-'
		}
	}, t.Name())

	p := fmt.Sprintf("t%d-%s", keySeq.Add(1), name)
	if len(p) > 64 {
		p = p[:64]
	}

	return p
}

// optionsFor builds session.Options pointed at an already-running server,
// for tests that need a raw client sharing the store's backend.
func optionsFor(t *testing.T, s *miniredis.Miniredis) session.Options {
	t.Helper()

	return session.Options{Redis: session.RedisOptions{Addr: s.Addr(), Prefix: testPrefix(t)}}
}

func testOptions(t *testing.T) session.Options {
	t.Helper()

	return optionsFor(t, testServer(t))
}

func newTestStore(t *testing.T, opts session.Options) session.Store {
	t.Helper()

	st, err := sredis.New(opts)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})

	return st
}

// rawClient dials addr directly, bypassing the store, for scaffolding
// (planting corrupt records, asserting raw TTLs). addr must match the
// store's own Redis.Addr to observe the same backend.
func rawClient(t *testing.T, addr string) *goredis.Client {
	t.Helper()

	c := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = c.Close() })

	return c
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	addr := testServer(t).Addr()

	for name, opts := range map[string]session.Options{
		"negative TTL":   {TTL: -time.Second},
		"bad prefix":     {Redis: session.RedisOptions{Addr: addr, Prefix: "has space"}},
		"scheme in addr": {Redis: session.RedisOptions{Addr: "redis://localhost:6379"}},
	} {
		if _, err := sredis.New(opts); !errors.Is(err, session.ErrInvalidOptions) {
			t.Fatalf("%s: New err = %v, want ErrInvalidOptions", name, err)
		}
	}
}

func TestNew_pingFailure(t *testing.T) {
	t.Parallel()

	_, err := sredis.New(session.Options{
		Redis: session.RedisOptions{Addr: "127.0.0.1:1", Prefix: testPrefix(t)},
	})
	if err == nil {
		t.Fatal("New bad addr err = nil, want ping error")
	}

	if !strings.Contains(err.Error(), "ping") {
		t.Fatalf("New bad addr err = %v, want ping error", err)
	}
}

func TestCRUD_roundtrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)

	created, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if verr := session.ValidateID(created.ID); verr != nil {
		t.Fatalf("Create ID invalid: %v", verr)
	}

	got, err := st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get err = %v, want nil", err)
	}

	if got.ID != created.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, created.ID)
	}

	if !got.ExpiresAt.Equal(created.ExpiresAt) {
		t.Fatalf("Get ExpiresAt = %v, want %v", got.ExpiresAt, created.ExpiresAt)
	}

	created.Data["k"] = "v"
	if serr := st.Save(ctx, created); serr != nil {
		t.Fatalf("Save err = %v, want nil", serr)
	}

	got, err = st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after Save err = %v, want nil", err)
	}

	if got.Data["k"] != "v" {
		t.Fatalf("Get Data[k] = %v, want %q", got.Data["k"], "v")
	}

	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete err = %v, want nil", err)
	}

	if _, err := st.Get(ctx, created.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after Delete err = %v, want ErrNotFound", err)
	}

	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete missing err = %v, want nil", err)
	}
}

func TestGet_miss_ErrNotFound(t *testing.T) {
	t.Parallel()

	st := newTestStore(t, testOptions(t))
	if _, err := st.Get(t.Context(), session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get miss err = %v, want ErrNotFound", err)
	}
}

func TestExpiry(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	ctx := t.Context()
	opts := optionsFor(t, server)
	st := newTestStore(t, opts)
	raw := rawClient(t, opts.Redis.Addr)

	s, err := st.Create(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if _, err := st.Get(ctx, s.ID); err != nil {
		t.Fatalf("Get before expiry err = %v, want nil", err)
	}

	server.FastForward(3 * time.Second)

	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after expiry err = %v, want ErrNotFound", err)
	}

	if n := raw.Exists(ctx, opts.Redis.Prefix+":"+s.ID).Val(); n != 0 {
		t.Fatalf("Exists after expiry = %d, want 0", n)
	}
}

func TestCreate_defaultTTL(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t)) // TTL zero => DefaultTTL

	before := time.Now()
	s, err := st.Create(ctx, 0)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if s.ExpiresAt.Sub(before) < session.DefaultTTL-time.Minute ||
		s.ExpiresAt.Sub(before) > session.DefaultTTL+time.Minute {
		t.Fatalf("Create default ExpiresAt = %v, want ~now+%v", s.ExpiresAt, session.DefaultTTL)
	}
}

func TestCopyIndependence(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	s.Data["mut"] = "x"
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v, want nil", err)
	}

	if _, ok := got.Data["mut"]; ok {
		t.Fatal("Get Data contains caller mutation, want copy independence")
	}

	got.Data["mut2"] = "y"
	again, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v, want nil", err)
	}

	if _, ok := again.Data["mut2"]; ok {
		t.Fatal("Get Data contains prior-Get mutation, want copy independence")
	}
}

func TestSave_keepsExpiry(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t, opts.Redis.Addr)

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	s.Data["k"] = "v2"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v, want nil", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v, want nil", err)
	}

	if !got.ExpiresAt.Equal(s.ExpiresAt) {
		t.Fatalf("Save moved ExpiresAt to %v, want %v (absolute expiry preserved)", got.ExpiresAt, s.ExpiresAt)
	}

	if !got.CreatedAt.Equal(s.CreatedAt) {
		t.Fatalf("Save moved CreatedAt to %v, want %v", got.CreatedAt, s.CreatedAt)
	}

	ttl, err := raw.TTL(ctx, opts.Redis.Prefix+":"+s.ID).Result()
	if err != nil {
		t.Fatalf("TTL err = %v, want nil", err)
	}

	if ttl < 50*time.Minute || ttl > time.Hour {
		t.Fatalf("raw TTL = %v, want ~1h (not extended by Save)", ttl)
	}
}

func TestSave_missingCreates(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)

	id := session.NewID()
	before := time.Now()
	if err := st.Save(ctx, session.Session{ID: id, Data: map[string]any{"a": "b"}}); err != nil {
		t.Fatalf("Save missing err = %v, want nil", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after Save-missing err = %v, want nil", err)
	}

	if got.Data["a"] != "b" {
		t.Fatalf("Get Data[a] = %v, want %q", got.Data["a"], "b")
	}

	// Caller-provided expiry is ignored: store default applies.
	if got.ExpiresAt.Sub(before) < session.DefaultTTL-time.Minute ||
		got.ExpiresAt.Sub(before) > session.DefaultTTL+time.Minute {
		t.Fatalf("Save-missing ExpiresAt = %v, want ~now+%v", got.ExpiresAt, session.DefaultTTL)
	}
}

func TestGet_corruptRecord(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t, opts.Redis.Addr)

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if perr := raw.Set(ctx, opts.Redis.Prefix+":"+s.ID, []byte("{not-json"), time.Hour).Err(); perr != nil {
		t.Fatalf("plant garbage err = %v, want nil", perr)
	}

	_, err = st.Get(ctx, s.ID)
	if err == nil {
		t.Fatal("Get corrupt err = nil, want decode error")
	}

	if errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get corrupt err = %v, must not be ErrNotFound (fail closed)", err)
	}

	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Get corrupt err = %v, want decode error", err)
	}
}

func TestSave_corruptOverwrites(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t, opts.Redis.Addr)

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if perr := raw.Set(ctx, opts.Redis.Prefix+":"+s.ID, []byte("{not-json"), time.Hour).Err(); perr != nil {
		t.Fatalf("plant garbage err = %v, want nil", perr)
	}

	s.Data["k"] = "fresh"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save over corrupt err = %v, want nil", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after Save-over-corrupt err = %v, want nil", err)
	}

	if got.Data["k"] != "fresh" {
		t.Fatalf("Get Data[k] = %v, want %q", got.Data["k"], "fresh")
	}
}

func TestDefaultPrefix(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	ctx := t.Context()
	st := newTestStore(t, session.Options{Redis: session.RedisOptions{Addr: server.Addr()}})
	raw := rawClient(t, server.Addr())

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if n := raw.Exists(ctx, "sess:"+s.ID).Val(); n != 1 {
		t.Fatalf("Exists sess:%s... = %d, want 1 (default prefix)", s.ID[:8], n)
	}
}

func TestInvalidIDs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	for _, id := range []string{"", "short", "ZZZZ"} {
		if _, err := st.Get(ctx, id); !errors.Is(err, session.ErrInvalidID) {
			t.Fatalf("Get(%q) err = %v, want ErrInvalidID", id, err)
		}

		if err := st.Delete(ctx, id); !errors.Is(err, session.ErrInvalidID) {
			t.Fatalf("Delete(%q) err = %v, want ErrInvalidID", id, err)
		}
	}

	if err := st.Save(ctx, session.Session{ID: "bad"}); !errors.Is(err, session.ErrInvalidID) {
		t.Fatalf("Save bad ID err = %v, want ErrInvalidID", err)
	}
}

func TestContextCanceled(t *testing.T) {
	t.Parallel()

	st := newTestStore(t, testOptions(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := st.Create(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create canceled err = %v, want context.Canceled", err)
	}

	if _, err := st.Get(ctx, session.NewID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get canceled err = %v, want context.Canceled", err)
	}

	if err := st.Save(ctx, session.Session{ID: session.NewID()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save canceled err = %v, want context.Canceled", err)
	}

	if err := st.Delete(ctx, session.NewID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete canceled err = %v, want context.Canceled", err)
	}
}

func TestAfterClose(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if err := st.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	if err := st.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil (idempotent)", err)
	}

	if _, err := st.Create(ctx, time.Hour); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Create after Close err = %v, want ErrClosed", err)
	}

	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Get after Close err = %v, want ErrClosed", err)
	}

	if err := st.Save(ctx, s); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Save after Close err = %v, want ErrClosed", err)
	}

	if err := st.Delete(ctx, s.ID); !errors.Is(err, session.ErrClosed) {
		t.Fatalf("Delete after Close err = %v, want ErrClosed", err)
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			s, err := st.Create(ctx, time.Hour)
			if err != nil {
				t.Errorf("goroutine %d: Create err = %v", i, err)
				return
			}

			s.Data["i"] = i
			if err := st.Save(ctx, s); err != nil {
				t.Errorf("goroutine %d: Save err = %v", i, err)
				return
			}

			if _, err := st.Get(ctx, s.ID); err != nil {
				t.Errorf("goroutine %d: Get err = %v", i, err)
				return
			}

			if err := st.Delete(ctx, s.ID); err != nil {
				t.Errorf("goroutine %d: Delete err = %v", i, err)
			}
		}(i)
	}

	wg.Wait()
}
