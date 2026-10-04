package db

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// TestEdgeContextCancelled proves Begin/Complete/Forget honor a canceled
// context before touching the database.
func TestEdgeContextCancelled(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Begin canceled err = %v, want context.Canceled", err)
	}

	if err := s.Complete(ctx, "k", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete canceled err = %v, want context.Canceled", err)
	}

	if err := s.Forget(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Forget canceled err = %v, want context.Canceled", err)
	}
}

// TestEdgeReplayEmptyResult proves an empty completed result replays without
// error.
func TestEdgeReplayEmptyResult(t *testing.T) {
	t.Parallel()

	s := mustNew(t)
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
}

// TestEdgeForgetMissingKeyNil proves Forget on a missing key returns nil.
func TestEdgeForgetMissingKeyNil(t *testing.T) {
	t.Parallel()

	s := mustNew(t)

	if err := s.Forget(t.Context(), "never-seen"); err != nil {
		t.Fatalf("Forget(missing) err = %v, want nil", err)
	}
}
