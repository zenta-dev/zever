package kvstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestKeyAtMaxLenAccepted verifies the inclusive MaxKeyLen boundary.
func TestKeyAtMaxLenAccepted(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()
	key := strings.Repeat("k", MaxKeyLen)

	if err := s.Set(ctx, key, []byte("v"), time.Hour); err != nil {
		t.Fatalf("Set(max key) error = %v", err)
	}

	if _, ok, err := s.Get(ctx, key); err != nil || !ok {
		t.Fatalf("Get(max key) = (%v, %v), want hit", ok, err)
	}
}

// TestContextCancelled verifies every operation honors a cancelled context
// before touching the database.
func TestContextCancelled(t *testing.T) {
	t.Parallel()

	s := mustStore(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := s.Get(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get() error = %v, want context.Canceled", err)
	}

	if _, _, _, err := s.GetWithExpiry(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetWithExpiry() error = %v, want context.Canceled", err)
	}

	if err := s.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("Set() error = %v, want context.Canceled", err)
	}

	if _, err := s.SetNX(ctx, "k", []byte("v"), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("SetNX() error = %v, want context.Canceled", err)
	}

	if _, err := s.AddDelta(ctx, "k", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("AddDelta() error = %v, want context.Canceled", err)
	}

	if err := s.Delete(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Delete() error = %v, want context.Canceled", err)
	}

	if _, err := s.SweepExpired(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("SweepExpired() error = %v, want context.Canceled", err)
	}
}
