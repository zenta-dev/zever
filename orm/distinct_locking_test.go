package orm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// TestQueryDistinctDedup proves DISTINCT removes duplicate result rows
// against real SQLite: the widgets fixture is seeded with a duplicate of w1,
// so the plain SELECT returns four rows and DISTINCT returns three.
func TestQueryDistinctDedup(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, "w1", "Alpha", int64(10), "first"); err != nil {
		t.Fatalf("insert duplicate: %v", err)
	}

	all, err := From(widgets).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(all) != 4 {
		t.Fatalf("plain All = %d rows, want 4 (duplicate present)", len(all))
	}

	distinct, err := From(widgets).Distinct().All(ctx, conn)
	if err != nil {
		t.Fatalf("Distinct All: %v", err)
	}

	if len(distinct) != 3 {
		t.Fatalf("Distinct All = %d rows, want 3 (duplicate deduplicated)", len(distinct))
	}
}

// TestQueryDistinctBranchSafe proves Distinct is copy-on-write: branching a
// base Query leaves the base's distinct flag untouched.
func TestQueryDistinctBranchSafe(t *testing.T) {
	base := From(widgets)

	branch := base.Distinct()

	if base.distinct {
		t.Fatalf("base.distinct = true after branching, want false")
	}

	if !branch.distinct {
		t.Fatalf("branch.distinct = false, want true")
	}
}

// TestQueryLockingBranchSafe proves ForUpdate/ForShare/NoWait/SkipLocked are
// copy-on-write, and pins the documented last-wins rule for the mutually
// exclusive NOWAIT/SKIP LOCKED modifiers.
func TestQueryLockingBranchSafe(t *testing.T) {
	base := From(widgets)

	upd := base.ForUpdate()
	share := base.ForShare()

	if base.lock != LockNone {
		t.Fatalf("base.lock = %d after branching, want LockNone", base.lock)
	}

	if upd.lock != LockForUpdate {
		t.Fatalf("upd.lock = %d, want LockForUpdate", upd.lock)
	}

	if share.lock != LockForShare {
		t.Fatalf("share.lock = %d, want LockForShare", share.lock)
	}

	mod := base.ForUpdate().NoWait().SkipLocked()
	if mod.lock != LockForUpdate || mod.nowait || !mod.skipLocked {
		t.Fatalf("last-wins modifiers = (%d, nowait=%v, skip=%v), want (LockForUpdate, false, true)", mod.lock, mod.nowait, mod.skipLocked)
	}
}

// TestQueryLockingUnsupportedOnSQLite proves every locking form is a typed
// dialect.ErrUnsupportedByDialect on SQLite (which has no row-level
// locking) through the public All API -- never a panic and never SQL the
// server would reject.
func TestQueryLockingUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	for _, tc := range lockingForms {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.apply(From(widgets)).All(ctx, conn)
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("All err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestQueryLockingDistinctWithLockRejected proves the DISTINCT + row-lock
// combination is a typed, dialect-independent error even on a dialect that
// supports both features (Postgres) -- the SQL standard forbids the combo.
func TestQueryLockingDistinctWithLockRejected(t *testing.T) {
	ctx := t.Context()

	for _, tc := range []struct {
		name string
		q    Query[widget, *widget]
	}{
		{"DistinctThenForUpdate", From(widgets).Distinct().ForUpdate()},
		{"ForUpdateThenDistinct", From(widgets).ForUpdate().Distinct()},
		{"DistinctThenForShare", From(widgets).Distinct().ForShare()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.q.All(ctx, mockExec{dialectName: "postgres"})
			if !errors.Is(err, ErrLockingWithDistinct) {
				t.Fatalf("err = %v, want errors.Is(err, ErrLockingWithDistinct)", err)
			}
		})
	}
}

// TestQueryLockingModifierWithoutModeRejected proves NOWAIT/SKIP LOCKED
// without a preceding FOR UPDATE/FOR SHARE is a typed error rather than a
// silently ignored modifier.
func TestQueryLockingModifierWithoutModeRejected(t *testing.T) {
	ctx := t.Context()

	for _, tc := range []struct {
		name string
		q    Query[widget, *widget]
	}{
		{"NoWait", From(widgets).NoWait()},
		{"SkipLocked", From(widgets).SkipLocked()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.q.All(ctx, mockExec{dialectName: "postgres"})
			if !errors.Is(err, ErrLockingRequiresLockMode) {
				t.Fatalf("err = %v, want errors.Is(err, ErrLockingRequiresLockMode)", err)
			}
		})
	}
}

// TestQueryLockingRejectedOnCount proves Count/Exists do not silently drop a
// row-lock request: the lock is meaningless for an aggregate and surfaces as
// a typed error.
func TestQueryLockingRejectedOnCount(t *testing.T) {
	ctx := t.Context()

	if _, err := From(widgets).ForUpdate().Count(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrLockingNotSelect) {
		t.Fatalf("Count err = %v, want errors.Is(err, ErrLockingNotSelect)", err)
	}

	if _, err := From(widgets).ForShare().SkipLocked().Exists(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrLockingNotSelect) {
		t.Fatalf("Exists err = %v, want errors.Is(err, ErrLockingNotSelect)", err)
	}
}
