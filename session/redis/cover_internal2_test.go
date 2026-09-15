package redis

// White-box cover tests: direct store construction over dedicated
// clients. External tests cannot swap the pooled client, so
// fault-injection scenarios live here.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/session"
)

// failSetHook fails only SET commands against a live server: GET
// succeeds, so Save reaches its write and exercises the save-set
// error branch hermetically.
type failSetHook struct{}

func (failSetHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (failSetHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "set" {
			return errors.New("hook: set refused")
		}
		return next(ctx, cmd)
	}
}

func (failSetHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func TestCoverSaveSetFails(t *testing.T) {
	t.Parallel()

	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(srv.Close)

	c := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	c.AddHook(failSetHook{})

	st := &store{client: c, prefix: "cov", ttl: time.Minute}
	ctx := context.Background()

	if err := st.Save(ctx, session.Session{ID: session.NewID()}); err == nil {
		t.Fatal("Save(hook-refused set) = nil, want error")
	}
}
