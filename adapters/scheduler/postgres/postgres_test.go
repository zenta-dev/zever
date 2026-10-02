package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/job"
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
		var dup *job.DuplicateJobError
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
