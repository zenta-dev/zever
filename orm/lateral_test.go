package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestLateralCapabilityInterfacesCompile guards the two in-tree dialects
// against signature drift: both must satisfy dialect.LateralJoinDialect.
func TestLateralCapabilityInterfacesCompile(_ *testing.T) {
	var (
		_ dialect.LateralJoinDialect = sqlite.New()
		_ dialect.LateralJoinDialect = postgres.New()
	)
}

// TestLateralTruthTable pins the capability truth table directly: Postgres
// yes, SQLite no.
func TestLateralTruthTable(t *testing.T) {
	if !postgres.New().SupportsLateral() {
		t.Fatal("postgres.SupportsLateral() = false, want true")
	}

	if sqlite.New().SupportsLateral() {
		t.Fatal("sqlite.SupportsLateral() = true, want false (no LATERAL syntax)")
	}
}

// TestLateralSQLiteGated proves the SQLite no-LATERAL gap is a typed
// dialect.ErrUnsupportedByDialect through the public All API -- never a
// panic or a silently-degraded join -- for both lateral builders.
func TestLateralSQLiteGated(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)

	_, err := CrossLateralOn(From(widgets), inner).All(ctx, mockExec{dialectName: "sqlite"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("CrossLateralOn.All on sqlite err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	_, err = LeftLateralOn(From(widgets), inner).All(ctx, mockExec{dialectName: "sqlite"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("LeftLateralOn.All on sqlite err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestLateralRenderPostgres is the correlated top-N-per-group statement
// rendered against a LATERAL-capable dialect: the inner query's WHERE
// correlates to the outer widgets table, its ORDER BY/LIMIT render INSIDE
// the lateral parentheses, and the outer ORDER BY qualifies to the left
// table. Postgres placeholder numbering is asserted too.
func TestLateralRenderPostgres(t *testing.T) {
	inner := From(widgetOrders).
		Where(orderWidgetID.EqOuter(Outer(widgetID))).
		OrderBy(woAmount.Desc()).
		Limit(2)

	j := CrossLateralOn(From(widgets), inner).OrderBy(widgetID.Asc())

	q, args, err := j.render(postgres.New())
	if err != nil {
		t.Fatalf("render err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widgets"."quantity", "widgets"."bio", ` +
		`"widget_orders"."id", "widget_orders"."widget_id", "widget_orders"."amount" ` +
		`FROM "widgets" CROSS JOIN LATERAL ` +
		`(SELECT "id", "widget_id", "amount" FROM "widget_orders" WHERE "widget_id" = "widgets"."id" ` +
		`ORDER BY "amount" DESC LIMIT $1) AS "widget_orders" ORDER BY "widgets"."id" ASC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 1 || args[0] != 2 {
		t.Fatalf("args = %#v, want [2] (the inner LIMIT; the marker binds nothing)", args)
	}
}

// TestLateralRenderLeftPostgres renders the LEFT JOIN LATERAL ... ON TRUE
// shape and its Row2[A, Option[B]] contract's SQL.
func TestLateralRenderLeftPostgres(t *testing.T) {
	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(1)

	j := LeftLateralOn(From(widgets), inner)

	q, _, err := j.render(postgres.New())
	if err != nil {
		t.Fatalf("render err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widgets"."quantity", "widgets"."bio", ` +
		`"widget_orders"."id", "widget_orders"."widget_id", "widget_orders"."amount" ` +
		`FROM "widgets" LEFT JOIN LATERAL ` +
		`(SELECT "id", "widget_id", "amount" FROM "widget_orders" WHERE "widget_id" = "widgets"."id" LIMIT $1) ` +
		`AS "widget_orders" ON TRUE`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestLateralProjectionMismatchRejected proves the typed render-time error
// for an inner query whose explicit Columns projection differs from B's
// canonical column list -- the invariant the positional Row2 scan depends on.
func TestLateralProjectionMismatchRejected(t *testing.T) {
	inner := From(widgetOrders).Columns(woAmount.Col())

	_, _, err := CrossLateralOn(From(widgets), inner).render(postgres.New())
	if err == nil {
		t.Fatalf("render err = nil, want a full-column-list projection error")
	}

	if !strings.Contains(err.Error(), "full column list") {
		t.Fatalf("err = %v, want the full-column-list error", err)
	}
}

// TestLateralMismatchedOuterRejected proves a marker naming a table other
// than the left table is rejected at execution time, never rendered as a
// dangling column.
func TestLateralMismatchedOuterRejected(t *testing.T) {
	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(orderID)))

	_, _, err := CrossLateralOn(From(widgets), inner).render(postgres.New())
	if err == nil {
		t.Fatalf("render err = nil, want a mismatched-correlation error")
	}

	if !strings.Contains(err.Error(), "does not match the enclosing query's table") {
		t.Fatalf("err = %v, want the mismatched-table error", err)
	}
}

// lateralScanDB is a db.DB over fixed stubRows that reports the postgres
// dialect name (which supports LATERAL) and records the last
// SQL text -- enough to exercise the full All scan path for a lateral join
// without a live LATERAL-capable server.
type lateralScanDB struct {
	mockExec

	rows  *stubRows
	query string
}

func (l *lateralScanDB) Query(_ context.Context, query string, _ ...any) (db.Rows, error) {
	l.query = query

	return l.rows, nil
}

func (l *lateralScanDB) Dialect() string { return "postgres" }

// TestLateralChainModifiers proves the lateral chain methods compose and
// render: Where/OrderBy/OrderByInner/Limit/Offset bodies run and the chained
// statement carries them all.
func TestLateralChainModifiers(t *testing.T) {
	inner := func() Query[widgetOrder, *widgetOrder] {
		return From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)
	}

	cross := CrossLateralOn(From(widgets), inner()).
		Where(widgetID.Eq("w1")).
		OrderBy(widgetID.Asc()).
		OrderByInner(woAmount.Desc()).
		Limit(3).Offset(1)

	q, _, err := cross.render(postgres.New())
	if err != nil {
		t.Fatalf("cross render: %v", err)
	}

	for _, frag := range []string{"CROSS JOIN LATERAL", "ORDER BY", "LIMIT", "OFFSET"} {
		if !strings.Contains(q, frag) {
			t.Fatalf("cross query %q missing %q", q, frag)
		}
	}

	left := LeftLateralOn(From(widgets), inner()).
		Where(widgetID.Eq("w1")).
		OrderBy(widgetID.Asc()).
		OrderByInner(woAmount.Desc()).
		Limit(3).Offset(1)

	ql, _, err := left.render(postgres.New())
	if err != nil {
		t.Fatalf("left render: %v", err)
	}

	for _, frag := range []string{"LEFT JOIN LATERAL", "ON TRUE", "OFFSET"} {
		if !strings.Contains(ql, frag) {
			t.Fatalf("left query %q missing %q", ql, frag)
		}
	}
}

// TestLateralSelfAlias proves a self-lateral (inner table == outer table)
// suffixed the derived-table alias so the two FROM entries never collide.
func TestLateralSelfAlias(t *testing.T) {
	inner := From(widgets).Limit(1)

	q, _, err := CrossLateralOn(From(widgets), inner).render(postgres.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(q, `AS "widgets_lateral"`) {
		t.Fatalf("query %q missing the suffixed self-lateral alias", q)
	}
}

// TestLateralStreamDrainsAndBreaks proves Stream yields scanned rows, stops
// on consumer break, and surfaces execution failures.
func TestLateralStreamDrainsAndBreaks(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).OrderBy(woAmount.Desc()).Limit(2)

	newConn := func() *lateralScanDB {
		return &lateralScanDB{rows: &stubRows{
			cols: []string{"widgets.id", "widgets.name", "widgets.quantity", "widgets.bio", "widget_orders.id", "widget_orders.widget_id", "widget_orders.amount"},
			values: [][]any{
				{"w1", "Alpha", int64(10), "first", "o2", "w1", int64(15)},
				{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)},
			},
		}}
	}

	var got []Row2[widget, widgetOrder]

	for row, err := range CrossLateralOn(From(widgets), inner).Stream(ctx, newConn()) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		got = append(got, row)
	}

	if len(got) != 2 || got[0].B.ID != "o2" {
		t.Fatalf("Stream drained %+v, want 2 rows starting o2", got)
	}

	broken := 0

	for _, err := range CrossLateralOn(From(widgets), inner).Stream(ctx, newConn()) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		broken++

		break
	}

	if broken != 1 {
		t.Fatalf("broke after %d rows, want 1", broken)
	}

	if err := drainJoinStream(CrossLateralOn(From(widgets), inner).Stream(ctx, fakeDB{})); err == nil {
		t.Fatal("Stream on an unresolvable dialect succeeded, want an error")
	}

	boom := errors.New("boom")

	stub := &ormStubDB{mockExec: mockExec{dialectName: "postgres"}, queryErr: boom}

	if err := drainJoinStream(CrossLateralOn(From(widgets), inner).Stream(ctx, stub)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := CrossLateralOn(From(widgets), inner).All(ctx, stub); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}
}

// TestLateralExplainPaths proves Explain/ExplainAnalyze run the lateral
// statement under EXPLAIN, failing closed on an unresolvable dialect.
func TestLateralExplainPaths(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(1)
	e := mockExec{dialectName: "postgres"}

	if _, err := CrossLateralOn(From(widgets), inner).Explain(ctx, e); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if _, err := CrossLateralOn(From(widgets), inner).ExplainAnalyze(ctx, e); err != nil {
		t.Fatalf("ExplainAnalyze: %v", err)
	}

	if _, err := LeftLateralOn(From(widgets), inner).Explain(ctx, e); err != nil {
		t.Fatalf("Left Explain: %v", err)
	}

	if _, err := LeftLateralOn(From(widgets), inner).ExplainAnalyze(ctx, e); err != nil {
		t.Fatalf("Left ExplainAnalyze: %v", err)
	}

	if _, err := CrossLateralOn(From(widgets), inner).Explain(ctx, fakeDB{}); err == nil {
		t.Fatal("Explain on an unresolvable dialect succeeded, want an error")
	}

	if _, err := LeftLateralOn(From(widgets), inner).ExplainAnalyze(ctx, fakeDB{}); err == nil {
		t.Fatal("ExplainAnalyze on an unresolvable dialect succeeded, want an error")
	}
}

// TestLateralCrossAllScans proves the CROSS JOIN LATERAL All path scans A's
// columns then B's from ONE row per result, in projection order, via the
// shared scanJoinRow (no per-row second query).
func TestLateralCrossAllScans(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).OrderBy(woAmount.Desc()).Limit(2)

	conn := &lateralScanDB{rows: &stubRows{
		cols: []string{"widgets.id", "widgets.name", "widgets.quantity", "widgets.bio", "widget_orders.id", "widget_orders.widget_id", "widget_orders.amount"},
		values: [][]any{
			{"w1", "Alpha", int64(10), "first", "o2", "w1", int64(15)},
			{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)},
		},
	}}

	got, err := CrossLateralOn(From(widgets), inner).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	if got[0].A.ID != "w1" || got[0].B.ID != "o2" || got[0].B.Amount != 15 {
		t.Fatalf("row 0 = %+v, want w1/o2/15", got[0])
	}

	if got[1].A.ID != "w1" || got[1].B.ID != "o1" || got[1].B.Amount != 10 {
		t.Fatalf("row 1 = %+v, want w1/o1/10", got[1])
	}

	if !strings.Contains(conn.query, "CROSS JOIN LATERAL") {
		t.Fatalf("executed SQL = %q, want a CROSS JOIN LATERAL clause", conn.query)
	}
}

// TestLateralLeftAllScansNone proves the LEFT JOIN LATERAL scan path turns
// an all-NULL inner side into Option[B]{}.IsSome() == false, never a
// same-shaped zero-valued B{}.
func TestLateralLeftAllScansNone(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)

	conn := &lateralScanDB{rows: &stubRows{
		cols: []string{"widgets.id", "widgets.name", "widgets.quantity", "widgets.bio", "widget_orders.id", "widget_orders.widget_id", "widget_orders.amount"},
		values: [][]any{
			{"w4", "Delta", int64(15), nil, nil, nil, nil},
		},
	}}

	got, err := LeftLateralOn(From(widgets), inner).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}

	if got[0].A.ID != "w4" {
		t.Fatalf("A.ID = %q, want w4", got[0].A.ID)
	}

	if got[0].B.IsSome() {
		t.Fatalf("B = %+v, want None (unmatched lateral side)", got[0].B)
	}

	if !strings.Contains(conn.query, "LEFT JOIN LATERAL") || !strings.Contains(conn.query, "ON TRUE") {
		t.Fatalf("executed SQL = %q, want LEFT JOIN LATERAL ... ON TRUE", conn.query)
	}
}

// TestLateralOrderByInnerQualifies proves OrderByInner qualifies the term to
// the derived-table alias, not the inner entity's real table name.
func TestLateralOrderByInnerQualifies(t *testing.T) {
	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(1)

	j := CrossLateralOn(From(widgets), inner).
		OrderBy(widgetID.Asc()).
		OrderByInner(woAmount.Desc())

	q, _, err := j.render(postgres.New())
	if err != nil {
		t.Fatalf("render err = %v, want nil", err)
	}

	if !strings.Contains(q, `ORDER BY "widgets"."id" ASC, "widget_orders"."amount" DESC`) {
		t.Fatalf("query = %q, want the inner term qualified to the derived-table alias", q)
	}
}

// TestLateralInnerOrderMismatchRejected proves an inner query projecting
// the right columns in the wrong order is a typed construction-time error.
func TestLateralInnerOrderMismatchRejected(t *testing.T) {
	inner := From(widgetOrders).Columns(woAmount.Col(), orderWidgetID.Col(), orderID.Col())

	_, _, err := CrossLateralOn(From(widgets), inner).render(postgres.New())
	if err == nil {
		t.Fatal("render err = nil, want an order-mismatch error")
	}

	if !strings.Contains(err.Error(), "in order") {
		t.Fatalf("err = %v, want the full-column-list-in-order error", err)
	}
}

// TestLeftLateralRenderErrorPaths proves LeftLateralOn's render fails
// closed on a bad inner projection and a gated dialect.
func TestLeftLateralRenderErrorPaths(t *testing.T) {
	inner := From(widgetOrders).Columns(woAmount.Col())

	_, _, err := LeftLateralOn(From(widgets), inner).render(postgres.New())
	if err == nil {
		t.Fatal("render err = nil, want a full-column-list projection error")
	}

	good := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(1)

	_, _, err = LeftLateralOn(From(widgets), good).render(sqlite.New())
	if err == nil {
		t.Fatal("render on sqlite err = nil, want a LATERAL capability error")
	}
}

// TestLeftLateralStreamDrains proves LeftLateralJoin2.Stream yields scanned
// rows and surfaces execution failures.
func TestLeftLateralStreamDrains(t *testing.T) {
	ctx := t.Context()

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)

	conn := &lateralScanDB{rows: &stubRows{
		cols: []string{"widgets.id", "widgets.name", "widgets.quantity", "widgets.bio", "widget_orders.id", "widget_orders.widget_id", "widget_orders.amount"},
		values: [][]any{
			{"w4", "Delta", int64(15), nil, nil, nil, nil},
		},
	}}

	var n int

	for row, err := range LeftLateralOn(From(widgets), inner).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.B.IsSome() {
			t.Fatalf("row.B = %+v, want None", row.B)
		}

		n++
	}

	if n != 1 {
		t.Fatalf("Stream yielded %d rows, want 1", n)
	}

	if err := drainJoinStream(LeftLateralOn(From(widgets), inner).Stream(ctx, fakeDB{})); err == nil {
		t.Fatal("Stream on an unresolvable dialect succeeded, want an error")
	}

	boom := errors.New("boom")
	stub := &ormStubDB{mockExec: mockExec{dialectName: "postgres"}, queryErr: boom}

	if err := drainJoinStream(LeftLateralOn(From(widgets), inner).Stream(ctx, stub)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}
}

// TestLateralCollectErrorPaths drives scan, iteration and close failures
// through the shared lateralCollect tail.
func TestLateralCollectErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)
	j := CrossLateralOn(From(widgets), inner)

	scanErr := &ormStubDB{
		mockExec: mockExec{dialectName: "postgres"},
		rows:     &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)}}, scanErr: boom},
	}

	if _, err := j.All(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{
		mockExec: mockExec{dialectName: "postgres"},
		rows: &stubRows{
			values:  [][]any{{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)}},
			iterErr: boom,
		},
	}

	if _, err := j.All(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "postgres"}, rows: &stubRows{closeErr: boom}}

	if _, err := j.All(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := j.All(ctx, fakeDB{}); err == nil {
		t.Fatal("All on an unresolvable dialect succeeded, want an error")
	}
}

// TestLateralStreamErrorPaths drives render, query, scan, iteration and
// close failures through the shared lateral stream tail.
func TestLateralStreamErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID))).Limit(2)
	j := CrossLateralOn(From(widgets), inner)

	if err := drainJoinStream(j.Stream(ctx, mockExec{dialectName: "sqlite"})); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Stream err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "postgres"}, rows: rows}
	}

	queryErr := newStub(&stubRows{})
	queryErr.queryErr = boom

	if err := drainJoinStream(j.Stream(ctx, queryErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	scanErr := newStub(&stubRows{
		values:  [][]any{{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)}},
		scanErr: boom,
	})

	if err := drainJoinStream(j.Stream(ctx, scanErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{
		values:  [][]any{{"w1", "Alpha", int64(10), "first", "o1", "w1", int64(10)}},
		iterErr: boom,
	})

	if err := drainJoinStream(j.Stream(ctx, iterErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if err := drainJoinStream(j.Stream(ctx, closeErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}
}
