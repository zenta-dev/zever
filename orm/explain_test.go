package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// TestQueryExplain runs Explain against a real SQLite database and checks
// the plan text looks like the bytecode listing SQLite's EXPLAIN produces.
func TestQueryExplain(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	lines, err := From(widgets).Where(widgetID.Eq("w1")).Explain(ctx, conn)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if len(lines) == 0 {
		t.Fatalf("Explain returned no plan lines")
	}

	joined := strings.Join(lines, "\n")

	for _, want := range []string{"Init", "OpenRead", "ResultRow"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("plan text missing %q:\n%s", want, joined)
		}
	}
}

// TestQueryExplainAnalyzeUnsupportedOnSQLite proves the SQLite EXPLAIN
// ANALYZE gap is a typed dialect.ErrUnsupportedByDialect through the public
// API -- never silently rendering a statement SQLite would reject.
func TestQueryExplainAnalyzeUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := From(widgets).ExplainAnalyze(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("ExplainAnalyze on sqlite = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// planRows is a hand-rolled db.Rows yielding a fixed set of already
// materialized values, used to observe the exact SQL Explain issues without
// a real database.
type planRows struct {
	cols     []string
	values   [][]any
	idx      int
	scanErr  error
	iterErr  error
	closeErr error
	colsErr  error
}

func (r *planRows) Next() bool {
	r.idx++

	return r.idx <= len(r.values)
}

func (r *planRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	for i := range dest {
		if i < len(r.values[r.idx-1]) {
			if d, ok := dest[i].(*any); ok {
				*d = r.values[r.idx-1][i]
			}
		}
	}

	return nil
}

func (r *planRows) Close() error               { return r.closeErr }
func (r *planRows) Err() error                 { return r.iterErr }
func (r *planRows) Columns() ([]string, error) { return r.cols, r.colsErr }

// planDB is a db.DB that records the query text it was handed and
// replies with one canned plan row, used to assert Explain/ExplainAnalyze's
// SQL passthrough (EXPLAIN / EXPLAIN ANALYZE prefix, rendered SELECT text,
// and the dialect's placeholders) end to end.
type planDB struct {
	db.DB
	query string
	rows  *planRows
}

func (r *planDB) Query(_ context.Context, query string, _ ...any) (db.Rows, error) {
	r.query = query

	if r.rows != nil {
		return r.rows, nil
	}

	return &planRows{cols: []string{"QUERY PLAN"}, values: [][]any{{"Seq Scan on widgets"}}}, nil
}

func (r *planDB) Dialect() string { return "postgres" }

// TestQueryExplainPassthroughSQL proves Explain issues
// "EXPLAIN " + the exact SELECT Stream/All would run (same WHERE args,
// same placeholders), and returns the plan lines.
func TestQueryExplainPassthroughSQL(t *testing.T) {
	ctx := t.Context()

	rec := &planDB{}

	lines, err := From(widgets).Where(widgetID.Eq("w1")).Explain(ctx, rec)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if !strings.HasPrefix(rec.query, "EXPLAIN ") {
		t.Fatalf("issued query %q, want an EXPLAIN prefix", rec.query)
	}

	if !strings.Contains(rec.query, `FROM "widgets"`) {
		t.Fatalf("issued query %q, want the rendered SELECT over widgets", rec.query)
	}

	if !strings.Contains(rec.query, `"id" = $1`) {
		t.Fatalf("issued query %q, want the WHERE placeholder passed through with postgres $1 numbering", rec.query)
	}

	if len(lines) != 1 || lines[0] != "Seq Scan on widgets" {
		t.Fatalf("Explain lines = %v, want [Seq Scan on widgets]", lines)
	}
}

// TestQueryExplainAnalyzePassthroughSQL proves ExplainAnalyze issues
// "EXPLAIN ANALYZE " + the rendered SELECT on a dialect that supports it.
func TestQueryExplainAnalyzePassthroughSQL(t *testing.T) {
	ctx := t.Context()

	rec := &planDB{}

	lines, err := From(widgets).Where(widgetID.Eq("w1")).ExplainAnalyze(ctx, rec)
	if err != nil {
		t.Fatalf("ExplainAnalyze: %v", err)
	}

	if !strings.HasPrefix(rec.query, "EXPLAIN ANALYZE ") {
		t.Fatalf("issued query %q, want an EXPLAIN ANALYZE prefix", rec.query)
	}

	if len(lines) != 1 || lines[0] != "Seq Scan on widgets" {
		t.Fatalf("ExplainAnalyze lines = %v, want [Seq Scan on widgets]", lines)
	}
}

// TestQueryExplainUnsupportedDialect proves Explain surfaces the
// resolveDialect error before issuing anything.
func TestQueryExplainUnsupportedDialect(t *testing.T) {
	ctx := t.Context()

	_, err := From(widgets).Explain(ctx, fakeDB{})
	if err == nil {
		t.Fatalf("Explain with an unsupported dialect name succeeded, want an error")
	}
}

// TestExplainRenderErrorWrapped proves a render-time failure (here a row
// lock SQLite rejects) is wrapped with the Explain tag, for both Explain
// and ExplainAnalyze.
func TestExplainRenderErrorWrapped(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := From(widgets).ForUpdate().Explain(ctx, conn)
	if err == nil {
		t.Fatalf("Explain with FOR UPDATE on sqlite succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "Query.Explain") {
		t.Fatalf("err = %v, want it wrapped with the Query.Explain tag", err)
	}

	_, err = From(widgets).ForUpdate().ExplainAnalyze(ctx, conn)
	if err == nil {
		t.Fatalf("ExplainAnalyze with FOR UPDATE on sqlite succeeded, want an error")
	}

	// The closure-level gate also fires under ExplainAnalyze once the
	// ANALYZE capability gate passes: DISTINCT + lock is rejected on
	// postgres too.
	rec := &planDB{}

	_, err = From(widgets).Distinct().ForUpdate().ExplainAnalyze(ctx, rec)
	if !errors.Is(err, ErrLockingWithDistinct) {
		t.Fatalf("err = %v, want errors.Is(err, ErrLockingWithDistinct)", err)
	}
}

// TestExplainExecQueryError proves an EXPLAIN execution failure is wrapped
// with the Explain tag.
func TestExplainExecQueryError(t *testing.T) {
	ctx := t.Context()

	// Force the exec path to fail by handing explainExec a db whose Query
	// errors.
	_, err := explainExec(ctx, errQueryDB{}, false, "Query.Explain", "SELECT 1", nil)
	if err == nil {
		t.Fatalf("explainExec with a failing Query succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "Query.Explain") {
		t.Fatalf("err = %v, want the Query.Explain tag", err)
	}
}

// TestExplainExecErrorPaths covers every explainExec failure: Columns
// error, Scan error, rows.Err and Close error -- plus the nil-cell skip.
func TestExplainExecErrorPaths(t *testing.T) {
	ctx := t.Context()

	newExec := func(rows *planRows) *planDB { return &planDB{rows: rows} }

	if _, err := From(widgets).Explain(ctx, newExec(&planRows{colsErr: errors.New("cols boom")})); err == nil {
		t.Fatal("Explain with failing Columns succeeded, want an error")
	} else if !strings.Contains(err.Error(), "Query.Explain") {
		t.Fatalf("err = %v, want the Query.Explain tag", err)
	}

	if _, err := From(widgets).Explain(ctx, newExec(&planRows{
		cols:    []string{"a"},
		values:  [][]any{{"x"}},
		scanErr: errors.New("scan boom"),
	})); err == nil {
		t.Fatal("Explain with failing Scan succeeded, want an error")
	} else if !strings.Contains(err.Error(), "scan") {
		t.Fatalf("err = %v, want the scan tag", err)
	}

	if _, err := From(widgets).Explain(ctx, newExec(&planRows{iterErr: errors.New("iter boom")})); err == nil {
		t.Fatal("Explain with failing rows.Err succeeded, want an error")
	}

	if _, err := From(widgets).Explain(ctx, newExec(&planRows{closeErr: errors.New("close boom")})); err == nil {
		t.Fatal("Explain with failing Close succeeded, want an error")
	}

	// Nil cells are skipped, never rendered as "<nil>".
	lines, err := From(widgets).Explain(ctx, newExec(&planRows{
		cols:   []string{"a", "b"},
		values: [][]any{{"Seq Scan", nil}},
	}))
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if len(lines) != 1 || lines[0] != "Seq Scan" {
		t.Fatalf("lines = %v, want [Seq Scan] (nil cell skipped)", lines)
	}
}

// TestExplainMutatePassthrough proves the shared mutation twin issues
// EXPLAIN + the rendered statement through the same gate and exec path.
func TestExplainMutatePassthrough(t *testing.T) {
	ctx := t.Context()

	rec := &planDB{}

	lines, err := explainMutate(ctx, rec, false, "Update.Explain", func(d dialect.Dialect) (string, []any, error) {
		return `UPDATE "widgets" SET "name" = ` + d.Placeholder(1), []any{"x"}, nil
	})
	if err != nil {
		t.Fatalf("explainMutate: %v", err)
	}

	if !strings.HasPrefix(rec.query, "EXPLAIN ") {
		t.Fatalf("issued query %q, want an EXPLAIN prefix", rec.query)
	}

	if len(lines) != 1 || lines[0] != "Seq Scan on widgets" {
		t.Fatalf("lines = %v, want the canned plan row", lines)
	}

	if _, err := explainMutate(ctx, rec, true, "Update.Explain", func(dialect.Dialect) (string, []any, error) {
		return "", nil, errors.New("render boom")
	}); err == nil {
		t.Fatal("explainMutate with a failing render succeeded, want an error")
	}
}
