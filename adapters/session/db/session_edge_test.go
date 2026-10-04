package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
