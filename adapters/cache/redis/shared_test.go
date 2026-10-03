package redis

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/cache"
)

// TestShared_sameAddr_sameUnderlyingClient proves two cache batteries opened
// against the same Redis addr share one go-redis client (and thus one pool),
// and that closing one borrower does not close the client a peer still holds.
func TestShared_sameAddr_sameUnderlyingClient(t *testing.T) {
	s := miniredis.RunT(t)

	first, err := New(cache.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}

	second, err := New(cache.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}

	a, ok := first.(*redisAdapter)
	if !ok {
		t.Fatalf("first New() returned %T, want *redisAdapter", first)
	}

	b, ok := second.(*redisAdapter)
	if !ok {
		t.Fatalf("second New() returned %T, want *redisAdapter", second)
	}

	if a.client != b.client {
		t.Fatal("two batteries on the same addr got different clients, want a shared pool")
	}

	if err := first.Close(t.Context()); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	// The peer still holds a reference: the shared client stays live.
	if err := second.Set(t.Context(), "k", []byte("v"), 0); err != nil {
		t.Fatalf("second Set() after peer close error = %v", err)
	}

	if err := second.Close(t.Context()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if err := a.client.Ping(context.Background()).Err(); !errors.Is(err, goredis.ErrClosed) {
		t.Errorf("Ping() after all releases = %v, want ErrClosed", err)
	}
}
