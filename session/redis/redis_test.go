package redis_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	zredis "github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/session"
	sredis "github.com/zenta-dev/zever/session/redis"
)

// Shared hermetic server for all tests.
//
// New threads through the internal/redis shared Pool singleton, so every
// test must dial the same address: per-test miniredis instances on distinct
// ports would thrash the singleton (each New closes the previous client).
// Isolation comes from unique key prefixes per test instead.
var (
	testMini *miniredis.Miniredis
	testAddr string
	keySeq   atomic.Int64
)

func TestMain(m *testing.M) {
	s, err := miniredis.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "miniredis start:", err)
		os.Exit(1)
	}

	testMini = s
	testAddr = s.Addr()

	code := m.Run()

	s.Close()

	os.Exit(code)
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

func testOptions(t *testing.T) session.Options {
	t.Helper()

	return session.Options{Redis: session.RedisOptions{Addr: testAddr, Prefix: testPrefix(t)}}
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

// rawClient dials the test server directly, bypassing the shared Pool, for
// scaffolding (planting corrupt records, asserting raw TTLs).
func rawClient(t *testing.T) *goredis.Client {
	t.Helper()

	c := goredis.NewClient(&goredis.Options{Addr: testAddr})
	t.Cleanup(func() { _ = c.Close() })

	return c
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string]session.Options{
		"negative TTL":   {TTL: -time.Second},
		"bad prefix":     {Redis: session.RedisOptions{Addr: testAddr, Prefix: "has space"}},
		"scheme in addr": {Redis: session.RedisOptions{Addr: "redis://localhost:6379"}},
	} {
		if _, err := sredis.New(opts); !errors.Is(err, session.ErrInvalidOptions) {
			t.Fatalf("%s: New err = %v, want ErrInvalidOptions", name, err)
		}
	}
}

// Sequential on purpose: a failing Addr replaces the shared Pool client, so
// it must not run alongside other tests. It restores the shared client
// afterwards for file-order independence.
func TestNew_pingFailure(t *testing.T) {
	defer func() {
		_ = zredis.Close()

		if _, rerr := sredis.New(testOptions(t)); rerr != nil {
			t.Errorf("restore New err = %v, want nil", rerr)
		}
	}()

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

	ctx := context.Background()
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
	if _, err := st.Get(context.Background(), session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get miss err = %v, want ErrNotFound", err)
	}
}

// Sequential on purpose: FastForward jumps the shared server clock, so it
// must not run alongside other tests.
func TestExpiry(t *testing.T) {
	ctx := context.Background()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t)

	s, err := st.Create(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	if _, err := st.Get(ctx, s.ID); err != nil {
		t.Fatalf("Get before expiry err = %v, want nil", err)
	}

	testMini.FastForward(3 * time.Second)

	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after expiry err = %v, want ErrNotFound", err)
	}

	if n := raw.Exists(ctx, opts.Redis.Prefix+":"+s.ID).Val(); n != 0 {
		t.Fatalf("Exists after expiry = %d, want 0", n)
	}
}

func TestCreate_defaultTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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

	ctx := context.Background()
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

	ctx := context.Background()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t)

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

	ctx := context.Background()
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

	ctx := context.Background()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t)

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

	ctx := context.Background()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	raw := rawClient(t)

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

	ctx := context.Background()
	st := newTestStore(t, session.Options{Redis: session.RedisOptions{Addr: testAddr}})
	raw := rawClient(t)

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

	ctx := context.Background()
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

	ctx, cancel := context.WithCancel(context.Background())
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

	ctx := context.Background()
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

// delayFirstGetHook delays exactly the first GET issued against key until
// release is closed, signaling on reached once that GET has landed on the
// server (its reply is already fixed, only the caller's continuation is
// paused). Every later GET against the same key (retries, other calls)
// passes straight through: armed flips false after the first hit.
type delayFirstGetHook struct {
	key     string
	armed   atomic.Bool
	reached chan struct{}
	release chan struct{}
}

func newDelayFirstGetHook(key string) *delayFirstGetHook {
	h := &delayFirstGetHook{key: key, reached: make(chan struct{}), release: make(chan struct{})}
	h.armed.Store(true)

	return h
}

func (h *delayFirstGetHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *delayFirstGetHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *delayFirstGetHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)

		args := cmd.Args()
		if len(args) >= 2 && fmt.Sprint(args[0]) == "get" && fmt.Sprint(args[1]) == h.key {
			if h.armed.CompareAndSwap(true, false) {
				close(h.reached)
				<-h.release
			}
		}

		return err
	}
}

// TestSave_atomicUnderStaleReadRace reproduces the read-modify-write gap
// documented on Save: a Save that read the session before a concurrent Save
// legitimately re-created it (after expiry) must not resurrect the stale
// generation's CreatedAt/ExpiresAt over the fresh one. Non-atomic
// GET-then-SET loses this race; an atomic Save must retry and see the fresh
// generation.
func TestSave_atomicUnderStaleReadRace(t *testing.T) {
	ctx := context.Background()
	opts := testOptions(t)
	st := newTestStore(t, opts)

	s, err := st.Create(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	client, err := zredis.New(zredis.Options{Addr: testAddr})
	if err != nil {
		t.Fatalf("zredis.New err = %v, want nil", err)
	}

	key := opts.Redis.Prefix + ":" + s.ID
	hook := newDelayFirstGetHook(key)
	client.AddHook(hook)

	var (
		wg       sync.WaitGroup
		staleErr error
	)

	wg.Add(1)

	go func() {
		defer wg.Done()

		staleErr = st.Save(ctx, session.Session{ID: s.ID, Data: map[string]any{"who": "stale"}})
	}()

	select {
	case <-hook.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stale Save's GET to land")
	}

	// Expire the original generation, then let a fully independent Save
	// observe it as missing and create a fresh generation in its place.
	testMini.FastForward(3 * time.Second)

	if serr := st.Save(ctx, session.Session{ID: s.ID, Data: map[string]any{"who": "fresh"}}); serr != nil {
		t.Fatalf("fresh Save err = %v, want nil", serr)
	}

	fresh, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get fresh err = %v, want nil", err)
	}

	// Release the stale Save now that the fresh generation exists.
	close(hook.release)
	wg.Wait()

	if staleErr != nil {
		t.Fatalf("stale Save err = %v, want nil", staleErr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after race err = %v, want nil (must not resurrect an already-expired generation)", err)
	}

	if !got.CreatedAt.Equal(fresh.CreatedAt) {
		t.Fatalf("Save clobbered the fresh generation: CreatedAt = %v, want %v (fresh generation, not the stale pre-expiry read)", got.CreatedAt, fresh.CreatedAt)
	}

	if !got.ExpiresAt.After(time.Now()) {
		t.Fatalf("Save resurrected a stale, already-past ExpiresAt = %v, want a future expiry from the fresh generation", got.ExpiresAt)
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
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
