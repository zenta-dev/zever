package redis

import (
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/core/session"
)

// deadStore builds a store dialed at a closed port: every command fails
// fast with connection refused, exercising transport-failure branches
// without touching the shared TestMain server.
func deadStore() *store {
	return &store{
		client: goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"}),
		prefix: "dead",
		ttl:    time.Minute,
	}
}

func TestCoverTransportFailures(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := deadStore()
	id := session.NewID()

	if _, err := st.Create(ctx, 0); err == nil {
		t.Fatal("Create(dead) = nil, want transport error")
	}
	if _, err := st.Get(ctx, id); err == nil {
		t.Fatal("Get(dead) = nil, want transport error")
	}
	if err := st.Save(ctx, session.Session{ID: id}); err == nil {
		t.Fatal("Save(dead) = nil, want transport error")
	}
	if err := st.Delete(ctx, id); err == nil {
		t.Fatal("Delete(dead) = nil, want transport error")
	}
}

func TestCoverRedactAddrUserinfo(t *testing.T) {
	t.Parallel()

	got := redactAddr("redis://user:s3cret@host:6379")
	if strings.Contains(got, "s3cret") {
		t.Errorf("redactAddr leaked password: %q", got)
	}
	if !strings.Contains(got, "xxxxx") {
		t.Errorf("redactAddr = %q, want masked password", got)
	}
}
