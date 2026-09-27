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

	"github.com/zenta-dev/zever/core/session"
)

// failSetHook fails only SET commands against a live server: GET
// succeeds, so Save reaches its write and exercises the save-set
// error branch hermetically. Save's SET now runs inside a MULTI/EXEC
// pipeline (see saveTx), so the pipeline hook is the one that actually
// sees it; the plain ProcessHook override stays as a safety net in case
// a SET ever reaches Save outside a pipeline.
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
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			if cmd.Name() == "set" {
				return errors.New("hook: set refused")
			}
		}

		return next(ctx, cmds)
	}
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
	ctx := t.Context()

	if err := st.Save(ctx, session.Session{ID: session.NewID()}); err == nil {
		t.Fatal("Save(hook-refused set) = nil, want error")
	}
}
