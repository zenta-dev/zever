//go:build leak

package container

import (
	"context"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestContainer_ShutdownSIGTERM_NoGoroutineLeak resolves the in-memory
// subset, delivers a real SIGTERM to the process, and verifies Close
// drains every background worker. Gated behind the `leak` tag like the
// other goleak tests: run with `go test -tags=leak ./container/`.
func TestContainer_ShutdownSIGTERM_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	c := New(nil, fakeProviders()...)

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

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal self SIGTERM: %v", err)
	}
	select {
	case <-sigCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SIGTERM delivery")
	}
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
