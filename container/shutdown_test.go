package container

import (
	"context"
	"errors"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// shutdownOrder records the order Close/Stop ran across seeded services.
// Close drains one service at a time on the calling goroutine, so the
// recording order is the shutdown order.
type shutdownOrder struct {
	mu    sync.Mutex
	names []string
}

func (o *shutdownOrder) add(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.names = append(o.names, name)
}

func (o *shutdownOrder) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.names)
}

// orderScheduler records Stop. It embeds scheduler.Scheduler so only the
// shutdown shape needs a body.
type orderScheduler struct {
	scheduler.Scheduler
	rec *shutdownOrder
}

func (s *orderScheduler) Stop() error {
	s.rec.add("scheduler")
	return nil
}

// orderCache records Close.
type orderCache struct {
	cache.Cache
	rec *shutdownOrder
}

func (c *orderCache) Close(context.Context) error {
	c.rec.add("cache")
	return nil
}

// orderQueue records Close.
type orderQueue struct {
	queue.Queue
	rec *shutdownOrder
}

func (q *orderQueue) Close() error {
	q.rec.add("queue")
	return nil
}

// sigtermDB is a db stub whose ping answers until Close, then fails: it
// models the database going away with the process, so Ready flips from
// nil to ErrDatabaseUnavailable across shutdown. It also records its
// close position in the shutdown order.
type sigtermDB struct {
	db.DB
	rec *shutdownOrder

	mu     sync.Mutex
	closed bool
}

func (d *sigtermDB) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errSigtermDBDown
	}
	return nil
}

func (d *sigtermDB) Close(context.Context) error {
	d.rec.add("db")
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// errSigtermDBDown is the ping failure a closed sigtermDB reports.
var errSigtermDBDown = errors.New("container test: db closed")

// awaitSIGTERM delivers one SIGTERM to this process and waits for ctx
// (from signal.NotifyContext) to observe it. The signal is caught by the
// notifier, so the test process survives it.
func awaitSIGTERM(ctx context.Context, t *testing.T) {
	t.Helper()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal self SIGTERM: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SIGTERM delivery")
	}
}

// TestContainer_Shutdown_UnderSIGTERM runs a resolved container under a
// real SIGTERM: readiness is true before the signal, Close drains in
// dependency order after it, and readiness flips to unavailable once the
// database is down. It is not parallel: process-directed signals and
// shutdown ordering do not compose with parallel package tests.
func TestContainer_Shutdown_UnderSIGTERM(t *testing.T) {
	c := New(testConfig(t))
	closeRelayContainer(t, c)

	rec := &shutdownOrder{}
	mustSeed(t, &c.scheduler, scheduler.Scheduler(&orderScheduler{rec: rec}))
	mustSeed(t, &c.cache, cache.Cache(&orderCache{rec: rec}))
	mustSeed(t, &c.queue, queue.Queue(&orderQueue{rec: rec}))
	mustSeed(t, &c.db, db.DB(&sigtermDB{rec: rec}))

	if err := c.Ready(t.Context()); err != nil {
		t.Fatalf("Ready() before SIGTERM = %v, want nil", err)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	awaitSIGTERM(sigCtx, t)
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close() after SIGTERM = %v, want nil", err)
	}

	if got, want := rec.snapshot(), []string{"scheduler", "cache", "queue", "db"}; !slices.Equal(got, want) {
		t.Errorf("shutdown order = %v, want %v (dependents first, leaves last, db after borrowers)", got, want)
	}

	if err := c.Ready(context.Background()); !errors.Is(err, ErrDatabaseUnavailable) {
		t.Errorf("Ready() after shutdown = %v, want ErrDatabaseUnavailable", err)
	}

	if err := c.Close(ctx); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}
