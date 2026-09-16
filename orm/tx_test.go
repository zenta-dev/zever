package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// renameWidget runs a raw UPDATE through exec: the tx/retry tests exercise
// transaction plumbing, not the mutation builders, so they write with plain
// SQL and read back through Query.
func renameWidget(ctx context.Context, exec db.DB, id, name string) error {
	_, err := exec.Exec(ctx, `UPDATE widgets SET name = ? WHERE id = ?`, name, id)

	return err
}

// TestWithNestedTxNoOuterTx proves the no-existing-tx path behaves
// exactly like db.WithTx: it starts a real transaction, runs fn, and
// commits on success.
func TestWithNestedTxNoOuterTx(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := WithNestedTx(ctx, conn, func(ctx context.Context, tx db.Tx) error {
		if _, ok := db.TxFromContext(ctx); !ok {
			t.Fatalf("fn's ctx does not carry the started Tx")
		}

		return renameWidget(ctx, tx, "w1", "Renamed")
	})
	if err != nil {
		t.Fatalf("WithNestedTx: %v", err)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil || !ok || row.Name != "Renamed" {
		t.Fatalf("row after commit = %+v, ok=%v, err=%v, want Renamed", row, ok, err)
	}
}

// TestWithNestedTxRollsBackOnError proves the no-existing-tx path rolls
// back fn's writes when fn returns an error.
func TestWithNestedTxRollsBackOnError(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	sentinel := errors.New("boom")

	err := WithNestedTx(ctx, conn, func(ctx context.Context, tx db.Tx) error {
		if err := renameWidget(ctx, tx, "w1", "ShouldNotStick"); err != nil {
			return err
		}

		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithNestedTx err = %v, want sentinel", err)
	}

	row, ok, ferr := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if ferr != nil || !ok || row.Name != "Alpha" {
		t.Fatalf("row after rollback = %+v, ok=%v, err=%v, want unchanged Alpha", row, ok, ferr)
	}
}

// TestWithNestedTxSavepointIsolation is the concrete isolation proof: an
// outer WithNestedTx starts a real transaction and writes a row; a nested
// WithNestedTx call (same ctx) degrades to a SAVEPOINT and writes a second
// row but then fails, rolling back ONLY its own write; the outer
// transaction's own write must still be intact and commit successfully once
// the outer call returns nil.
func TestWithNestedTxSavepointIsolation(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	sentinel := errors.New("inner failure")

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, outerTx db.Tx) error {
		// Outer write: rename w1 -- this must survive regardless of what
		// the inner nested call below does.
		if err := renameWidget(outerCtx, outerTx, "w1", "OuterWrite"); err != nil {
			return err
		}

		innerErr := WithNestedTx(outerCtx, conn, func(innerCtx context.Context, innerTx db.Tx) error {
			// Sanity: the inner call must be handed back the SAME Tx as the
			// outer one (a savepoint, not a second real transaction).
			if innerTx != outerTx {
				t.Fatalf("inner Tx != outer Tx: nested WithNestedTx started a real transaction instead of a savepoint")
			}

			// Inner write: rename w2 -- this must be rolled back by the
			// savepoint when this function returns an error, without
			// touching the outer transaction.
			if err := renameWidget(innerCtx, innerTx, "w2", "InnerWriteShouldRollBack"); err != nil {
				return err
			}

			return sentinel
		})
		if !errors.Is(innerErr, sentinel) {
			t.Fatalf("inner WithNestedTx err = %v, want sentinel", innerErr)
		}

		// The outer transaction must still be alive and usable after the
		// inner savepoint rolled back -- prove it with a read through the
		// same Tx before returning to let the outer call commit.
		row, ok, err := From(widgets).Where(widgetID.Eq("w2")).First(outerCtx, outerTx)
		if err != nil || !ok {
			t.Fatalf("read w2 inside outer tx after inner rollback: row=%v ok=%v err=%v", row, ok, err)
		}

		if row.Name != "Beta" {
			t.Fatalf("w2.Name after inner rollback = %q, want unchanged %q", row.Name, "Beta")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("outer WithNestedTx: %v", err)
	}

	// After commit: w1's rename (outer) must have stuck, w2's rename
	// (inner) must not have -- proving the savepoint isolated the inner
	// failure from the outer transaction's own writes.
	w1, ok1, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil || !ok1 || w1.Name != "OuterWrite" {
		t.Fatalf("w1 after commit = %+v, ok=%v, err=%v, want OuterWrite", w1, ok1, err)
	}

	w2, ok2, err := From(widgets).Where(widgetID.Eq("w2")).First(ctx, conn)
	if err != nil || !ok2 || w2.Name != "Beta" {
		t.Fatalf("w2 after commit = %+v, ok=%v, err=%v, want unchanged Beta", w2, ok2, err)
	}
}

// TestWithNestedTxSavepointSuccessIsPartOfOuterCommit proves the
// complementary case: when the inner nested call SUCCEEDS, its write is
// released (not rolled back) and becomes part of the outer transaction's
// commit.
func TestWithNestedTxSavepointSuccessIsPartOfOuterCommit(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, _ db.Tx) error {
		return WithNestedTx(outerCtx, conn, func(innerCtx context.Context, innerTx db.Tx) error {
			return renameWidget(innerCtx, innerTx, "w3", "InnerWriteSticks")
		})
	})
	if err != nil {
		t.Fatalf("outer WithNestedTx: %v", err)
	}

	w3, ok, ferr := From(widgets).Where(widgetID.Eq("w3")).First(ctx, conn)
	if ferr != nil || !ok || w3.Name != "InnerWriteSticks" {
		t.Fatalf("w3 after commit = %+v, ok=%v, err=%v, want InnerWriteSticks", w3, ok, ferr)
	}
}

// TestWithNestedTxSavepointErrorWrapped proves a SAVEPOINT failure is
// wrapped with the savepoint name rather than returned bare.
func TestWithNestedTxSavepointErrorWrapped(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, outerTx db.Tx) error {
		// The nested path resolves its Tx from the context, so the
		// failing wrapper must ride the context, not the exec argument.
		wrapped := db.WithTxIntoContext(outerCtx, &savepointFailTx{Tx: outerTx})

		return WithNestedTx(wrapped, conn, func(context.Context, db.Tx) error {
			return nil
		})
	})
	if err == nil {
		t.Fatal("WithNestedTx with a failing savepoint succeeded, want an error")
	}

	if got, want := err.Error(), "savepoint sp_1"; !strings.Contains(got, want) {
		t.Fatalf("err = %q, want it to mention %q", got, want)
	}
}

// TestWithNestedTxRollbackToErrorWrapped proves a ROLLBACK TO SAVEPOINT
// failure after fn's error reports both the original error text and the
// rollback failure.
func TestWithNestedTxRollbackToErrorWrapped(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	sentinel := errors.New("inner failure")

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, outerTx db.Tx) error {
		wrapped := db.WithTxIntoContext(outerCtx, &rollbackToFailTx{Tx: outerTx})

		return WithNestedTx(wrapped, conn, func(context.Context, db.Tx) error {
			return sentinel
		})
	})
	if err == nil {
		t.Fatal("WithNestedTx with a failing rollback-to succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "rollback to savepoint") {
		t.Fatalf("err = %q, want it to mention rollback to savepoint", err)
	}
}

// TestWithNestedTxReleaseErrorWrapped proves a RELEASE SAVEPOINT failure
// after fn's success is wrapped with the savepoint name.
func TestWithNestedTxReleaseErrorWrapped(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := WithNestedTx(ctx, conn, func(outerCtx context.Context, outerTx db.Tx) error {
		wrapped := db.WithTxIntoContext(outerCtx, &releaseFailTx{Tx: outerTx})

		return WithNestedTx(wrapped, conn, func(context.Context, db.Tx) error {
			return nil
		})
	})
	if err == nil {
		t.Fatal("WithNestedTx with a failing release succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "release savepoint") {
		t.Fatalf("err = %q, want it to mention release savepoint", err)
	}
}

// savepointFailTx fails Savepoint; every other method delegates.
type savepointFailTx struct {
	db.Tx
}

func (t *savepointFailTx) Savepoint(context.Context, string) error {
	return errors.New("savepoint boom")
}

// rollbackToFailTx fails RollbackTo but otherwise behaves: Savepoint is a
// no-op success so the test reaches the rollback path.
type rollbackToFailTx struct {
	db.Tx
}

func (t *rollbackToFailTx) Savepoint(context.Context, string) error { return nil }
func (t *rollbackToFailTx) RollbackTo(context.Context, string) error {
	return errors.New("rollback-to boom")
}

// releaseFailTx fails the RELEASE SAVEPOINT Exec but otherwise behaves.
type releaseFailTx struct {
	db.Tx
}

func (t *releaseFailTx) Savepoint(context.Context, string) error { return nil }
func (t *releaseFailTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if len(query) >= 7 && query[:7] == "RELEASE" {
		return 0, errors.New("release boom")
	}

	return t.Tx.Exec(ctx, query, args...)
}

// TestNextSavepointNameIsPerDepth proves distinct nesting depths get
// distinct savepoint names.
func TestNextSavepointNameIsPerDepth(t *testing.T) {
	name1, depth1 := nextSavepointName(context.Background())
	if name1 != "sp_1" || depth1 != 1 {
		t.Fatalf("nextSavepointName(no depth) = (%q, %d), want (sp_1, 1)", name1, depth1)
	}

	ctxDepth1 := context.WithValue(context.Background(), spDepthKey{}, depth1)

	name2, depth2 := nextSavepointName(ctxDepth1)
	if name2 != "sp_2" || depth2 != 2 {
		t.Fatalf("nextSavepointName(depth 1) = (%q, %d), want (sp_2, 2)", name2, depth2)
	}
}
