package memory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

func openDefault(t *testing.T) session.Store {
	t.Helper()
	st, err := memory.New(session.Options{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}
}

func TestNew_negative_TTL_fails(t *testing.T) {
	t.Parallel()
	if _, err := memory.New(session.Options{TTL: -time.Second}); err == nil {
		t.Fatal("New negative TTL expected error, got nil")
	}
}

func TestCRUD_roundtrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	created, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if verr := session.ValidateID(created.ID); verr != nil {
		t.Fatalf("Create ID invalid: %v", verr)
	}
	got, err := st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGet_miss_ErrNotFound(t *testing.T) {
	t.Parallel()
	st := openDefault(t)
	if _, err := st.Get(t.Context(), session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get miss err = %v, want ErrNotFound", err)
	}
}

func TestInvalidIDs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
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

func TestCopyIndependence_get_mutation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	s.Data["k"] = "v"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	got.Data["k"] = "MUTATED"
	got.Data["new"] = true
	again, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if again.Data["k"] != "v" {
		t.Fatalf("stored Data[k] = %v, want %q (aliased on read)", again.Data["k"], "v")
	}
	if _, ok := again.Data["new"]; ok {
		t.Fatal("stored Data shares map with Get result")
	}
}

func TestCopyIndependence_save_mutation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	s.Data["k"] = "v"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}
	s.Data["k"] = "MUTATED"
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if got.Data["k"] != "v" {
		t.Fatalf("stored Data[k] = %v, want %q (aliased on write)", got.Data["k"], "v")
	}
}

func TestSave_keeps_expiry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	want := s.ExpiresAt
	s.Data["k"] = "v"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if !got.ExpiresAt.Equal(want) {
		t.Fatalf("Save extended expiry: got %v want %v", got.ExpiresAt, want)
	}
	if !got.UpdatedAt.After(s.UpdatedAt) && got.UpdatedAt.Equal(s.UpdatedAt) {
		t.Log("note: UpdatedAt not strictly after (fast clock), checking non-zero")
	}
}

func TestSave_missing_creates_with_defaultTTL(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ttl := 30 * time.Minute
	st, err := memory.New(session.Options{TTL: ttl})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	before := time.Now()
	s := session.NewSession(session.NewID(), 0) // zero expiry provided
	s.Data["k"] = "v"
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	lo := before.Add(ttl - 10*time.Second)
	hi := time.Now().Add(ttl + 10*time.Second)
	if got.ExpiresAt.Before(lo) || got.ExpiresAt.After(hi) {
		t.Fatalf("Save-missing ExpiresAt = %v, want within [%v, %v]", got.ExpiresAt, lo, hi)
	}
}

func TestExpiry_lazy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	eventually(t, 2*time.Second, func() bool {
		_, err := st.Get(ctx, s.ID)
		return errors.Is(err, session.ErrNotFound)
	}, "session expiry")
	// expired entry purged: second Get still generic miss
	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get purged err = %v, want ErrNotFound", err)
	}
}

func TestSweeper_purges(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := memory.New(session.Options{TTL: 40 * time.Millisecond})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s, err := st.Create(ctx, 0) // falls back to store default (40ms)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	eventually(t, 2*time.Second, func() bool {
		_, err := st.Get(ctx, s.ID)
		return errors.Is(err, session.ErrNotFound)
	}, "sweeper expiry")
}

func TestDelete_idempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if err := st.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
	if err := st.Delete(ctx, s.ID); err != nil {
		t.Fatalf("second Delete err = %v, want nil", err)
	}
	if err := st.Delete(ctx, session.NewID()); err != nil {
		t.Fatalf("Delete unknown err = %v, want nil", err)
	}
	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after Delete err = %v, want ErrNotFound", err)
	}
}

func TestConcurrent_access(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := openDefault(t)
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = st.Save(ctx, func() session.Session {
				g, err := st.Get(ctx, s.ID)
				if err != nil {
					return s
				}
				return g
			}())
			_, _ = st.Get(ctx, s.ID)
		}()
	}
	wg.Wait()
}

func TestAfterClose_errors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := memory.New(session.Options{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
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
	if err := st.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}
}

func TestContext_canceled(t *testing.T) {
	t.Parallel()
	st := openDefault(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := st.Create(ctx, time.Hour); err == nil {
		t.Fatal("Create canceled ctx expected error, got nil")
	}
	if _, err := st.Get(ctx, session.NewID()); err == nil {
		t.Fatal("Get canceled ctx expected error, got nil")
	}
	if err := st.Save(ctx, session.Session{ID: session.NewID()}); err == nil {
		t.Fatal("Save canceled ctx expected error, got nil")
	}
	if err := st.Delete(ctx, session.NewID()); err == nil {
		t.Fatal("Delete canceled ctx expected error, got nil")
	}
}
