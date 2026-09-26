package redis

// TestSave_atomicUnderStaleReadRace lives in this internal test package
// (rather than redis_test) because it hooks the store's own *goredis.Client
// to delay the first GET issued by a Save call: since New now returns an
// independently owned client per store (no shared singleton to piggyback
// on), the hook must be installed on the exact client the store uses.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/session"
)

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
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	ctx := t.Context()
	prefix := "race"

	st := &store{prefix: prefix, ttl: 15 * time.Minute}

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	st.client = client

	s, err := st.Create(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	key := prefix + ":" + s.ID
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
	mr.FastForward(3 * time.Second)

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
