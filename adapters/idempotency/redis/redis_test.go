package redis

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/idempotency"
)

// keySeq keeps generated keys unique even within a single test's shared
// miniredis instance.
var keySeq atomic.Int64

func freshKey(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("k-%d", keySeq.Add(1))
}

// testServer starts a per-test miniredis instance, auto-closed via
// t.Cleanup.
func testServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	return miniredis.RunT(t)
}

// testOptions starts a per-test miniredis instance and returns options
// pointed at it.
func testOptions(t *testing.T) idempotency.Options {
	t.Helper()

	return idempotency.Options{Redis: idempotency.RedisOptions{Addr: testServer(t).Addr()}}
}

func newTestStore(t *testing.T) idempotency.Store {
	t.Helper()

	return newTestStoreWithServer(t, testServer(t))
}

func newTestStoreWithServer(t *testing.T, s *miniredis.Miniredis) idempotency.Store {
	t.Helper()

	opts := idempotency.Options{Redis: idempotency.RedisOptions{Addr: s.Addr()}}

	store, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})

	return store
}

func TestRedis_claimCompleteReplay(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp-1")
	want := []byte("result-1")

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if out.Replay {
		t.Fatalf("Begin Replay = true, want false (winner)")
	}

	if cerr := s.Complete(ctx, key, fp, want); cerr != nil {
		t.Fatalf("Complete err = %v, want nil", cerr)
	}

	out, err = s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin replay err = %v, want nil", err)
	}

	if !out.Replay {
		t.Fatalf("Begin Replay = false, want true")
	}

	if string(out.Result) != string(want) {
		t.Fatalf("Begin Result = %q, want %q", out.Result, want)
	}
}

func TestRedis_secondClaimantInProgress(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp")

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("Begin second err = %v, want ErrInProgress", err)
	}
}

func TestRedis_mismatchPending(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin mismatch err = %v, want ErrKeyMismatch", err)
	}
}

func TestRedis_mismatchDone(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, []byte("a"), []byte("res")); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Begin mismatch err = %v, want ErrKeyMismatch", err)
	}
}

func TestRedis_nilFingerprintCompat(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, nil, []byte("res")); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{})
	if err != nil {
		t.Fatalf("Begin replay err = %v, want nil", err)
	}

	if !out.Replay || string(out.Result) != "res" {
		t.Fatalf("Begin replay = %+v, want {true res}", out)
	}
}

func TestRedis_completeWithoutBegin(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp")

	if err := s.Complete(ctx, key, fp, []byte("upsert")); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if !out.Replay || string(out.Result) != "upsert" {
		t.Fatalf("Begin replay = %+v, want {true upsert}", out)
	}
}

func TestRedis_completeMismatchNoOverwrite(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, []byte("b"), []byte("evil")); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Complete mismatch err = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, key, []byte("a"), []byte("good")); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: []byte("a")})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if !out.Replay || string(out.Result) != "good" {
		t.Fatalf("Begin replay = %+v, want {true good}", out)
	}
}

func TestRedis_expiry(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	ctx := t.Context()
	s := newTestStoreWithServer(t, server)
	key := freshKey(t)
	fp := []byte("fp")

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp, TTL: 2 * time.Second}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	server.FastForward(3 * time.Second)

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin after expiry err = %v, want nil", err)
	}

	if out.Replay {
		t.Fatalf("Begin Replay = true, want false (reservation expired)")
	}
}

func TestRedis_forget(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp")

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, fp, []byte("res")); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	if err := s.Forget(ctx, key); err != nil {
		t.Fatalf("Forget err = %v, want nil", err)
	}

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin after Forget err = %v, want nil", err)
	}

	if out.Replay {
		t.Fatalf("Begin Replay = true, want false (forgotten)")
	}

	if err := s.Forget(ctx, key); err != nil {
		t.Fatalf("Forget existing err = %v, want nil", err)
	}

	if err := s.Forget(ctx, freshKey(t)); err != nil {
		t.Fatalf("Forget missing err = %v, want nil", err)
	}
}

func TestRedis_copyIndependence(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp")
	res := []byte("result")

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if err := s.Complete(ctx, key, fp, res); err != nil {
		t.Fatalf("Complete err = %v, want nil", err)
	}

	res[0] = 'X'

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if string(out.Result) != "result" {
		t.Fatalf("Begin Result = %q, want %q (caller mutation leaked)", out.Result, "result")
	}

	out.Result[0] = 'X'

	out2, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin err = %v, want nil", err)
	}

	if string(out2.Result) != "result" {
		t.Fatalf("Begin Result = %q, want %q (replay mutation leaked)", out2.Result, "result")
	}
}

func TestRedis_afterClose(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close second err = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Begin after Close err = %v, want ErrClosed", err)
	}

	if err := s.Complete(ctx, key, nil, []byte("r")); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Complete after Close err = %v, want ErrClosed", err)
	}

	if err := s.Forget(ctx, key); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Forget after Close err = %v, want ErrClosed", err)
	}
}

func TestRedis_invalidKeys(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)

	long := make([]byte, idempotency.MaxKeyLen+1)
	for i := range long {
		long[i] = 'k'
	}

	bad := map[string]string{
		"empty":   "",
		"newline": "a\nb",
		"control": "a\x00b",
		"toolong": string(long),
	}

	for name, key := range bad {
		if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Begin(%s) err = %v, want ErrInvalidKey", name, err)
		}

		if err := s.Complete(ctx, key, nil, []byte("r")); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Complete(%s) err = %v, want ErrInvalidKey", name, err)
		}

		if err := s.Forget(ctx, key); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Forget(%s) err = %v, want ErrInvalidKey", name, err)
		}
	}
}

func TestRedis_invalidOptions(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*idempotency.Options){
		"bad addr":     func(o *idempotency.Options) { o.Redis.Addr = "://bad" },
		"negative ttl": func(o *idempotency.Options) { o.TTL = -time.Second },
		"bad prefix":   func(o *idempotency.Options) { o.Redis.Prefix = "has space" },
	} {
		opts := testOptions(t)
		mutate(&opts)

		if _, err := New(opts); err == nil {
			t.Errorf("New(%s) err = nil, want error", name)
		}
	}
}

func TestRedis_concurrentSingleWinner(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	fp := []byte("fp")

	const n = 20

	var wg sync.WaitGroup

	wins := atomic.Int64{}
	inprog := atomic.Int64{}

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
			if err == nil && !out.Replay {
				wins.Add(1)

				return
			}

			if errors.Is(err, idempotency.ErrInProgress) {
				inprog.Add(1)

				return
			}

			t.Errorf("Begin err = %v out = %+v, want win or ErrInProgress", err, out)
		}()
	}

	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("winners = %d, want 1", wins.Load())
	}

	if inprog.Load() != n-1 {
		t.Fatalf("in-progress = %d, want %d", inprog.Load(), n-1)
	}
}

func TestRedis_fingerprintTooLarge_rejected(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s := newTestStore(t)
	key := freshKey(t)
	big := make([]byte, 5000)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: big}); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Begin oversize fp = %v, want ErrFingerprintTooLarge", err)
	}

	if err := s.Complete(ctx, key, big, []byte("r")); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Complete oversize fp = %v, want ErrFingerprintTooLarge", err)
	}
}
