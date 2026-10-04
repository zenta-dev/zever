package redis

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// TestEdgeReplayEmptyResult proves an empty completed result replays without
// error.
func TestEdgeReplayEmptyResult(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := t.Context()
	key := freshKey(t)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, key, nil, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	out, err := s.Begin(ctx, key, idempotency.BeginOptions{})
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

	s := newTestStore(t)

	if err := s.Forget(t.Context(), freshKey(t)); err != nil {
		t.Fatalf("Forget(missing) err = %v, want nil", err)
	}
}

// TestEdgeContextCancelled proves a canceled context is honored.
func TestEdgeContextCancelled(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.Begin(ctx, freshKey(t), idempotency.BeginOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Begin canceled err = %v, want context.Canceled", err)
	}
}
