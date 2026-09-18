package redis

// White-box cover tests: direct store construction over a dead client
// exercises the transport-failure branches that external tests, bound to
// the shared internal/redis singleton, cannot reach.

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// deadStore builds a store dialed at a closed port: every command fails
// fast with connection refused.
func deadStore() *store {
	return &store{
		client: goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"}),
		prefix: "dead",
	}
}

func TestCoverTransportFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := deadStore()

	if err := st.Revoke(ctx, "jti", time.Now().Add(time.Minute)); err == nil {
		t.Fatal("Revoke(dead) = nil, want transport error")
	}
	if _, err := st.IsRevoked(ctx, "jti"); err == nil {
		t.Fatal("IsRevoked(dead) = nil, want transport error")
	}
}
