//go:build leak

package container

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/zenta-dev/zever/config"
)

// Leak tests are gated behind the `leak` tag so `go test ./...` stays fast:
// run with `go test -tags=leak ./container/`. A package-level TestMain check
// is deliberately avoided: importing adapter packages starts third-party
// init-time workers (e.g. go.opencensus.io stats/view via geo/google) that
// outlive any single test. That worker is filtered below: it starts at init
// and is owned by the dependency, not by container code.

// leakOptions filters process-lifetime third-party init workers owned by
// dependencies, not by container code.
func leakOptions() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
	}
}

// TestContainer_Close_NoGoroutineLeak resolves the in-memory services that
// spawn background work (queue promoter, scheduler cron) and verifies Close
// drains them.
func TestContainer_Close_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	c := New(config.Default())

	for _, resolve := range []func() (any, error){
		func() (any, error) { return c.Cache() },
		func() (any, error) { return c.Queue() },
		func() (any, error) { return c.Scheduler() },
		func() (any, error) { return c.Job() },
	} {
		if _, err := resolve(); err != nil {
			t.Fatalf("resolve: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestContainer_Close_NoGoroutineLeak_Idempotent ensures a second Close
// leaks nothing either.
func TestContainer_Close_NoGoroutineLeak_Idempotent(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	c := New(config.Default())

	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}
	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestContainer_Close_NoGoroutineLeak_Empty verifies Close on an untouched
// container leaks nothing (nothing resolved, nothing to drain).
func TestContainer_Close_NoGoroutineLeak_Empty(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	c := New(config.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
