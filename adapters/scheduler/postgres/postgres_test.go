package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

func echoJob(_ context.Context, _ string) error { return nil }

func mustNew(t *testing.T, opts Options) *driver {
	t.Helper()

	if opts.PoolOptions.Path == "" && opts.PoolOptions.DSN == "" {
		// Fresh file per test: ":memory:" sqlite uses shared cache
		// (process-global), so fixed slot IDs would collide across
		// reruns (-count) and leak between parallel tests.
		opts.PoolOptions.Path = filepath.Join(t.TempDir(), "scheduler.db")
	}

	if opts.Owner == "" {
		opts.Owner = "owner-test"
	}

	if opts.Dispatcher == nil {
		opts.Dispatcher = &job.Dispatcher{Q: newStubQueue()}
	}

	s, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New returned %T, want *driver", s)
	}

	return d
}

func registerJobOnce(t *testing.T, name string) {
	t.Helper()

	if err := job.Register(name, echoJob); err != nil {
		var dup job.DuplicateJobError
		if !errors.As(err, &dup) {
			t.Fatalf("Register: %v", err)
		}
	}
}

func TestNewRequiresDispatcher(t *testing.T) {
	t.Parallel()

	_, err := New(Options{})
	if !errors.Is(err, scheduler.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestSchedulePersistsSlotRow(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	t.Cleanup(func() { _ = d.Close() })

	registerJobOnce(t, "sched-persist")

	ctx := t.Context()

	id, err := d.Schedule(ctx, "0 * * * *", "sched-persist", "args")
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	row, ok, err := d.load(ctx, d.slots[id])
	if err != nil || !ok {
		t.Fatalf("load = %v,%v,%v want row,true,nil", row, ok, err)
	}

	if row.JobName != "sched-persist" || row.LeaseOwner != "owner-1" {
		t.Errorf("row = %+v, want job sched-persist owned by owner-1", row)
	}
}

func TestLiveForeignLeaseSkipsAdopt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared.db")
	registerJobOnce(t, "sched-shared")

	d1 := mustNew(t, Options{PoolOptions: poolWithPath(path), Owner: "owner-1"})
	t.Cleanup(func() { _ = d1.Close() })

	if _, err := d1.Schedule(t.Context(), "0 * * * *", "sched-shared", nil); err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	if err := d1.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// owner-1's lease is still live: a second replica must not adopt it,
	// or both instances would fire the slot.
	d2 := mustNew(t, Options{PoolOptions: poolWithPath(path), Owner: "owner-2"})
	t.Cleanup(func() { _ = d2.Close() })

	if got := len(d2.Entries()); got != 0 {
		t.Fatalf("adopted %d live foreign slots, want 0", got)
	}
}

func TestExpiredLeaseReclaimedByPeer(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reclaim.db")
	registerJobOnce(t, "sched-reclaim")

	d1 := mustNew(t, Options{PoolOptions: poolWithPath(path), Owner: "owner-1"})
	t.Cleanup(func() { _ = d1.Close() })

	ctx := t.Context()

	if _, err := d1.Schedule(ctx, "0 * * * *", "sched-reclaim", nil); err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	if err := d1.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Expire owner-1's lease directly: deterministic, no clock waiting.
	past := time.Now().UTC().Add(-time.Minute)

	if _, err := d1.conn.Exec(ctx, `UPDATE "scheduler_slots" SET lease_expires_at = ?`, past.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	d2 := mustNew(t, Options{PoolOptions: poolWithPath(path), Owner: "owner-2"})
	t.Cleanup(func() { _ = d2.Close() })

	if got := len(d2.Entries()); got != 1 {
		t.Fatalf("adopted %d expired slots, want 1", got)
	}

	rows, err := querySlots(ctx, d2)
	if err != nil {
		t.Fatalf("query slots: %v", err)
	}

	if len(rows) != 1 || rows[0].LeaseOwner != "owner-2" {
		t.Fatalf("rows = %+v, want one slot owned by owner-2", rows)
	}
}

func TestRemoveDeletesSlotRow(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	t.Cleanup(func() { _ = d.Close() })

	registerJobOnce(t, "sched-remove")

	ctx := t.Context()

	id, err := d.Schedule(ctx, "0 * * * *", "sched-remove", nil)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	slot := d.slots[id]

	if err := d.Remove(id); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, ok, err := d.load(ctx, slot); err != nil || ok {
		t.Fatalf("load after Remove = %v,%v want missing", ok, err)
	}
}

// testCtxKey marks a request-scoped value a registration ctx carries.
type testCtxKey string

const testRequestKey testCtxKey = "request-key"

// blockingQueue blocks in Push until the call ctx is done, recording that
// the fire deadline (or Stop) canceled it.
type blockingQueue struct {
	entered chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingQueue() *blockingQueue {
	return &blockingQueue{entered: make(chan struct{}), done: make(chan struct{})}
}

func (b *blockingQueue) Push(ctx context.Context, _ string, _ queue.Payload, _ queue.Headers) error {
	b.once.Do(func() { close(b.entered) })
	<-ctx.Done()
	close(b.done)

	return ctx.Err()
}

func (b *blockingQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (b *blockingQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (b *blockingQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (b *blockingQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (b *blockingQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (b *blockingQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (b *blockingQueue) Close() error { return nil }

func (b *blockingQueue) Name() string { return "blocking-stub" }

// valueQueue records whether Push's ctx carried the registration request's
// value.
type valueQueue struct {
	mu       sync.Mutex
	sawValue bool
}

func (q *valueQueue) Push(ctx context.Context, _ string, _ queue.Payload, _ queue.Headers) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if ctx.Value(testRequestKey) != nil {
		q.sawValue = true
	}

	return nil
}

func (q *valueQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (q *valueQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (q *valueQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (q *valueQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (q *valueQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (q *valueQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (q *valueQueue) Close() error { return nil }

func (q *valueQueue) Name() string { return "value-stub" }

// findEntry returns the live cron entry for id.
func findEntry(t *testing.T, d *driver, id scheduler.EntryID) cron.Entry {
	t.Helper()

	for _, e := range d.cron.Entries() {
		//nolint:gosec // IDs originate from Schedule, a small positive cron sequence.
		if e.ID == cron.EntryID(id) {
			return e
		}
	}

	t.Fatalf("entry %d not found", id)

	return cron.Entry{}
}

func TestFireTimeout_BoundsHungDispatch(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-fire", FireTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { _ = d.Close() })

	registerJobOnce(t, "sched-fire-timeout")

	blk := newBlockingQueue()
	d.dispatcher = &job.Dispatcher{Q: blk}

	id, err := d.Schedule(t.Context(), "0 * * * *", "sched-fire-timeout", nil)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	findEntry(t, d, id).Job.Run()

	select {
	case <-blk.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fire ctx was not canceled by the fire timeout")
	}
}

func TestTickCtx_SchedulerRootedNotRequest(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-root"})
	t.Cleanup(func() { _ = d.Close() })

	registerJobOnce(t, "sched-root")

	vq := &valueQueue{}
	d.dispatcher = &job.Dispatcher{Q: vq}

	reqCtx := context.WithValue(t.Context(), testRequestKey, "request-value")

	id, err := d.Schedule(reqCtx, "0 * * * *", "sched-root", nil)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	findEntry(t, d, id).Job.Run()

	vq.mu.Lock()
	defer vq.mu.Unlock()

	if vq.sawValue {
		t.Fatal("fire ctx retained the registration request's values; want scheduler-rooted background ctx")
	}
}

func TestStop_AbortsInFlightFire(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-stop"})
	t.Cleanup(func() { _ = d.Close() })

	registerJobOnce(t, "sched-stop-abort")

	blk := newBlockingQueue()
	d.dispatcher = &job.Dispatcher{Q: blk}

	id, err := d.Schedule(t.Context(), "0 * * * *", "sched-stop-abort", nil)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	fireDone := make(chan struct{})

	go func() {
		defer close(fireDone)
		findEntry(t, d, id).Job.Run()
	}()

	select {
	case <-blk.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("fire did not reach the dispatcher")
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case <-blk.done:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight fire not aborted by Stop")
	}

	<-fireDone
}
