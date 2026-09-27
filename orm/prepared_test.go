package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// preparerSpy wraps a db.DB that implements db.Preparer (the sqlite
// adapter) and counts how many times orm routes a statement through
// Prepare rather than calling Query/Exec directly -- the probe contract
// documented on db.Preparer (probed the way db.Transactor is). Every count
// is one orm statement that reuses the adapter's prepared-statement cache
// instead of re-compiling SQL text per call.
// errPreparerMissing is returned by preparerSpy when the wrapped DB lacks
// db.Preparer. Unreachable in practice (tests wrap sqlite, which prepares);
// it exists so the comma-ok assertion has a typed failure.
var errPreparerMissing = errors.New("wrapped DB does not implement db.Preparer")

type preparerSpy struct {
	db.DB
	prepareCalls int
}

func (p *preparerSpy) Prepare(ctx context.Context, query string) (db.Stmt, error) {
	p.prepareCalls++

	preparer, ok := p.DB.(db.Preparer)
	if !ok {
		return nil, errPreparerMissing
	}

	return preparer.Prepare(ctx, query)
}

// plainWrapper is a db.DB that deliberately does NOT implement
// db.Preparer, so orm's probe falls back to Query/Exec exactly as before.
type plainWrapper struct{ db.DB }

// TestQueryRoutesThroughPreparer proves Query.All (and therefore First,
// which delegates to it) routes its SELECT through db.Preparer when the
// exec implements it.
func TestQueryRoutesThroughPreparer(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	spy := &preparerSpy{DB: conn}

	got, err := From(widgets).Where(widgetID.Eq("w2")).All(ctx, spy)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("All = %v, want exactly w2", got)
	}

	if spy.prepareCalls == 0 {
		t.Fatalf("All did not route through db.Preparer (prepareCalls = 0)")
	}
}

// TestCountRoutesThroughPreparer proves Query.Count's own SELECT executes
// through the prepared path too.
func TestCountRoutesThroughPreparer(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	spy := &preparerSpy{DB: conn}

	n, err := From(widgets).Where(widgetQty.Gt(15)).Count(ctx, spy)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}

	if spy.prepareCalls == 0 {
		t.Fatalf("Count did not route through db.Preparer (prepareCalls = 0)")
	}
}

// TestExecQueryRoutesThroughPreparer proves execQuery routes DML through
// the prepared path when the exec implements it.
func TestExecQueryRoutesThroughPreparer(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	spy := &preparerSpy{DB: conn}

	n, err := execQuery(ctx, spy, `UPDATE widgets SET name = ? WHERE id = ?`, []any{"Prepped", "w1"})
	if err != nil {
		t.Fatalf("execQuery: %v", err)
	}

	if n != 1 {
		t.Fatalf("execQuery rows = %d, want 1", n)
	}

	if spy.prepareCalls != 1 {
		t.Fatalf("Prepare calls = %d, want 1", spy.prepareCalls)
	}
}

// TestQueryRowsPrepareError proves a Prepare failure surfaces from
// queryRows rather than falling back silently.
func TestQueryRowsPrepareError(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := queryRows(ctx, &preparerSpy{DB: conn}, "SELECT 1 FROM \x00", nil)
	if err == nil {
		t.Fatalf("queryRows with an unpreparable statement succeeded, want an error")
	}
}

// TestExecQueryPrepareError proves a Prepare failure surfaces from
// execQuery.
func TestExecQueryPrepareError(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := execQuery(ctx, &preparerSpy{DB: conn}, "UPDATE \x00 SET x = 1", nil)
	if err == nil {
		t.Fatalf("execQuery with an unpreparable statement succeeded, want an error")
	}
}

// TestQueryFallsBackWhenNotPreparer proves a db.DB that does not implement
// db.Preparer gets Query/Exec called directly -- the pre-prepared behavior
// unchanged.
func TestQueryFallsBackWhenNotPreparer(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	w := plainWrapper{DB: conn}

	got, err := From(widgets).Where(widgetID.Eq("w2")).All(ctx, w)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("All = %v, want exactly w2", got)
	}

	_, err = w.Exec(ctx, `DELETE FROM widgets WHERE id = ?`, "w2")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	n, err := From(widgets).Count(ctx, w)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}

	// execQuery and queryRows fall back to Exec/Query directly.
	_, err = execQuery(ctx, w, `UPDATE widgets SET name = ? WHERE id = ?`, []any{"X", "w1"})
	if err != nil {
		t.Fatalf("execQuery fallback: %v", err)
	}

	rows, err := queryRows(ctx, w, `SELECT id FROM widgets`, nil)
	if err != nil {
		t.Fatalf("queryRows fallback: %v", err)
	}

	_ = rows.Close()
}

// errQueryDB is a non-Preparer db.DB whose Query always fails, proving
// queryRows surfaces the execution error.
type errQueryDB struct{ db.DB }

func (e errQueryDB) Query(context.Context, string, ...any) (db.Rows, error) {
	return nil, errors.New("query boom")
}

func (e errQueryDB) Dialect() string { return "sqlite" }

// TestQueryRowsQueryError proves a statement-execution failure surfaces
// from queryRows.
func TestQueryRowsQueryError(t *testing.T) {
	ctx := t.Context()

	_, err := queryRows(ctx, errQueryDB{}, `SELECT id FROM widgets`, nil)
	if err == nil {
		t.Fatalf("queryRows with a failing Query succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "query boom") {
		t.Fatalf("queryRows error = %v, want the execution failure", err)
	}
}

// TestCountScanError proves a COUNT scan failure is wrapped with the scan
// tag rather than returned bare.
func TestCountScanError(t *testing.T) {
	ctx := t.Context()

	_, err := From(widgets).Count(ctx, &stubDB{rows: &stubRows{
		cols:    []string{"count"},
		values:  [][]any{{"not-a-number"}},
		scanErr: errors.New("scan boom"),
	}})
	if err == nil {
		t.Fatalf("Count with a failing scan succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "scan") {
		t.Fatalf("count error not wrapped: %v", err)
	}
}
