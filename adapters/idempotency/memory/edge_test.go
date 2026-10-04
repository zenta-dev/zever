package memory

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// edgeStore opens a fresh in-memory store with defaults.
func edgeStore(t *testing.T) idempotency.Store {
	t.Helper()

	s, err := New(idempotency.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

// TestEdgeReplayEmptyResult proves a completed empty result replays as an
// empty result, not an error.
func TestEdgeReplayEmptyResult(t *testing.T) {
	t.Parallel()

	s := edgeStore(t)
	ctx := t.Context()

	if _, err := s.Begin(ctx, "edge-empty", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "edge-empty", nil, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	out, err := s.Begin(ctx, "edge-empty", idempotency.BeginOptions{})
	if err != nil {
		t.Fatalf("replay Begin: %v", err)
	}

	if !out.Replay {
		t.Fatal("Replay = false, want true")
	}

	if len(out.Result) != 0 {
		t.Fatalf("Result = %q, want empty", out.Result)
	}
}

// TestEdgeForgetMissingKeyNil proves Forget on a never-seen key succeeds.
func TestEdgeForgetMissingKeyNil(t *testing.T) {
	t.Parallel()

	s := edgeStore(t)

	if err := s.Forget(t.Context(), "never-seen"); err != nil {
		t.Fatalf("Forget(missing) err = %v, want nil", err)
	}
}

// TestEdgeDoubleCloseIdempotent proves Close is idempotent.
func TestEdgeDoubleCloseIdempotent(t *testing.T) {
	t.Parallel()

	s := edgeStore(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}

// TestEdgeBeginEmptyKeyRejected proves the empty key fails closed.
func TestEdgeBeginEmptyKeyRejected(t *testing.T) {
	t.Parallel()

	s := edgeStore(t)

	if _, err := s.Begin(t.Context(), "", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Fatalf("Begin(empty) err = %v, want ErrInvalidKey", err)
	}
}
