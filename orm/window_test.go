package orm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func init() {
	// sqlite-3.27 is the pre-3.28 floor: window frames arrived in SQLite
	// 3.28.0, so the frame-gate test can prove GROUPS is a typed error
	// there while ROWS passes on the default dialect.
	old, err := sqlite.NewWithVersion("3.27.2")
	if err != nil {
		panic(err) //nolint:forbidigo // test-only literal; the version cannot fail to parse
	}

	if err := dialect.Register("sqlite-3.27", func() dialect.Dialect { return old }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration
	}
}

// TestWindowFuncConstructors proves the window expression constructors
// build the expected erased expression values.
func TestWindowFuncConstructors(t *testing.T) {
	if got, want := RowNumber[widget](), (WindowExpr[widget]{fn: WinRowNumber, alias: "row_number"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("RowNumber() = %+v, want %+v", got, want)
	}

	if got, want := Rank[widget](), (WindowExpr[widget]{fn: WinRank, alias: "rank"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Rank() = %+v, want %+v", got, want)
	}

	if got, want := DenseRank[widget](), (WindowExpr[widget]{fn: WinDenseRank, alias: "dense_rank"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("DenseRank() = %+v, want %+v", got, want)
	}

	if got, want := Lead[widget](widgetName), (WindowExpr[widget]{fn: WinLead, col: "name", alias: "lead_name"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Lead() = %+v, want %+v", got, want)
	}

	if got, want := Lag[widget](widgetName), (WindowExpr[widget]{fn: WinLag, col: "name", alias: "lag_name"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Lag() = %+v, want %+v", got, want)
	}

	if got, want := NTile[widget](3), (WindowExpr[widget]{fn: WinNTile, value: 3, alias: "ntile"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("NTile(3) = %+v, want %+v", got, want)
	}

	if got, want := SumOver[widget](widgetQty), (WindowExpr[widget]{agg: &Aggregate{Func: AggSum, Column: "quantity"}, alias: "sum_quantity"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("SumOver() = %+v, want %+v", got, want)
	}

	for _, tc := range []struct {
		name string
		got  WindowExpr[widget]
		want WindowExpr[widget]
	}{
		{"AvgOver", AvgOver[widget](widgetQty), WindowExpr[widget]{agg: &Aggregate{Func: AggAvg, Column: "quantity"}, alias: "avg_quantity"}},
		{"MinOver", MinOver[widget](widgetQty), WindowExpr[widget]{agg: &Aggregate{Func: AggMin, Column: "quantity"}, alias: "min_quantity"}},
		{"MaxOver", MaxOver[widget](widgetQty), WindowExpr[widget]{agg: &Aggregate{Func: AggMax, Column: "quantity"}, alias: "max_quantity"}},
		{"SumNullableOver", SumNullableOver[widget](windowQtyNullable), WindowExpr[widget]{agg: &Aggregate{Func: AggSum, Column: "quantity"}, alias: "sum_quantity"}},
		{"AvgNullableOver", AvgNullableOver[widget](windowQtyNullable), WindowExpr[widget]{agg: &Aggregate{Func: AggAvg, Column: "quantity"}, alias: "avg_quantity"}},
		{"MinNullableOver", MinNullableOver[widget](windowQtyNullable), WindowExpr[widget]{agg: &Aggregate{Func: AggMin, Column: "quantity"}, alias: "min_quantity"}},
		{"MaxNullableOver", MaxNullableOver[widget](windowQtyNullable), WindowExpr[widget]{agg: &Aggregate{Func: AggMax, Column: "quantity"}, alias: "max_quantity"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Fatalf("%s() = %+v, want %+v", tc.name, tc.got, tc.want)
			}
		})
	}

	if got, want := CountOver[widget](), (WindowExpr[widget]{agg: &Aggregate{Func: AggCount}, alias: "count"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("CountOver() = %+v, want %+v", got, want)
	}
}

// windowQtyNullable is a nullable view of the quantity column for the
// nullable aggregate-window constructor tests.
var windowQtyNullable = NewNullableColumn[widget, int64]("widgets", "quantity")

// TestWindowQueryChainModifiers proves WindowQuery's Where/Offset chain
// methods compose: a doubly-filtered, offset window query runs against
// real SQLite and skips the first row.
func TestWindowQueryChainModifiers(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var got []string

	err := From(widgets).
		Select(RowNumber[widget]().Over(nil, []OrderTerm[widget]{widgetQty.Asc()})).
		Where(widgetQty.Gt(int64(5))).
		Where(widgetName.Neq("Gamma")).
		Limit(10).
		Offset(1).
		Scan(ctx, conn, func(r Row) error {
			var (
				id, name string
				qty, rn  int64
				bio      Option[string]
			)

			if err := r.Scan(&id, &name, &qty, &bio, &rn); err != nil {
				return err
			}

			got = append(got, id)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != 1 || got[0] != "w2" {
		t.Fatalf("ids = %v, want [w2] (w3 filtered out, offset skips w1)", got)
	}
}

// TestWindowExprOverDoesNotMutateBase proves Over attaches the window
// clause copy-on-write: reusing a base expression across branches is safe.
func TestWindowExprOverDoesNotMutateBase(t *testing.T) {
	base := SumOver[widget](widgetQty)

	partitioned := base.Over([]AnyColumn[widget]{widgetName.Col()}, nil)

	if len(base.over.partition) != 0 {
		t.Fatalf("base.over.partition mutated by branching: %v", base.over.partition)
	}

	if len(partitioned.over.partition) != 1 || partitioned.over.partition[0].Name() != "name" {
		t.Fatalf("partitioned.over.partition = %v, want [name]", partitioned.over.partition)
	}
}

// TestWindowExprFrameConstructors proves RowsBetween/RangeBetween/
// GroupsBetween record the frame mode and bounds copy-on-write, so a base
// expression can be reused across differently-framed branches.
func TestWindowExprFrameConstructors(t *testing.T) {
	base := SumOver[widget](widgetQty).Over([]AnyColumn[widget]{widgetName.Col()}, []OrderTerm[widget]{widgetQty.Asc()})

	rows := base.RowsBetween(UnboundedPreceding(), CurrentRow())
	if rows.over.frameMode != FrameRows {
		t.Fatalf("RowsBetween frameMode = %d, want FrameRows", rows.over.frameMode)
	}

	if rows.over.frameStart.Kind != BoundUnboundedPreceding || rows.over.frameEnd.Kind != BoundCurrentRow {
		t.Fatalf("RowsBetween bounds = (%+v, %+v), want (UnboundedPreceding, CurrentRow)", rows.over.frameStart, rows.over.frameEnd)
	}

	if base.over.frameMode != FrameNone {
		t.Fatalf("base.over.frameMode mutated by RowsBetween: %d", base.over.frameMode)
	}

	rng := base.RangeBetween(Preceding(1), Following(1))
	if rng.over.frameMode != FrameRange || rng.over.frameStart.N != 1 || rng.over.frameEnd.N != 1 {
		t.Fatalf("RangeBetween frame = (%d, %+v, %+v), want (FrameRange, 1 preceding, 1 following)", rng.over.frameMode, rng.over.frameStart, rng.over.frameEnd)
	}

	grp := base.GroupsBetween(CurrentRow(), UnboundedFollowing())
	if grp.over.frameMode != FrameGroups || grp.over.frameEnd.Kind != BoundUnboundedFollowing {
		t.Fatalf("GroupsBetween frame = (%d, %+v, %+v), want (FrameGroups, current row, unbounded following)", grp.over.frameMode, grp.over.frameStart, grp.over.frameEnd)
	}
}

// windowRow is the scan shape of TestWindowQueryScanRoundTrip's select
// list: widgets' four columns followed by the window expressions.
type windowRow struct {
	ID      string
	Name    string
	Qty     int64
	Bio     Option[string]
	RowNum  int64
	Rank    int64
	Lead    Option[string]
	Lag     Option[string]
	Tile    int64
	SumOver int64
}

// TestWindowQueryScanRoundTrip proves the whole window surface against real
// SQLite: ROW_NUMBER, RANK, LEAD/LAG, NTILE, and an aggregate over a
// window, all selected together and scanned positionally (entity columns
// first, then expressions in Select order).
func TestWindowQueryScanRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var got []windowRow

	err := From(widgets).
		Select(
			RowNumber[widget]().Over(nil, []OrderTerm[widget]{widgetQty.Asc()}),
			Rank[widget]().Over(nil, []OrderTerm[widget]{widgetQty.Desc()}),
			Lead[widget](widgetName).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}),
			Lag[widget](widgetName).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}),
			NTile[widget](2).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}),
			SumOver[widget](widgetQty).Over(nil, nil),
		).
		OrderBy(widgetQty.Asc()).
		Scan(ctx, conn, func(r Row) error {
			var rw windowRow
			if err := r.Scan(&rw.ID, &rw.Name, &rw.Qty, &rw.Bio, &rw.RowNum, &rw.Rank, &rw.Lead, &rw.Lag, &rw.Tile, &rw.SumOver); err != nil {
				return err
			}

			got = append(got, rw)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Scan returned %d rows, want 3", len(got))
	}

	// Rows are ordered by quantity ascending: w1(10), w2(20), w3(30).
	want := []windowRow{
		{ID: "w1", Name: "Alpha", Qty: 10, RowNum: 1, Rank: 3, Lead: Some("Beta"), Lag: None[string](), Tile: 1, SumOver: 60},
		{ID: "w2", Name: "Beta", Qty: 20, RowNum: 2, Rank: 2, Lead: Some("Gamma"), Lag: Some("Alpha"), Tile: 1, SumOver: 60},
		{ID: "w3", Name: "Gamma", Qty: 30, RowNum: 3, Rank: 1, Lead: None[string](), Lag: Some("Beta"), Tile: 2, SumOver: 60},
	}

	for i, w := range want {
		g := got[i]

		if g.ID != w.ID || g.RowNum != w.RowNum || g.Rank != w.Rank || g.Tile != w.Tile || g.SumOver != w.SumOver {
			t.Fatalf("got[%d] = %+v, want %+v (row_number/rank/tile/sum mismatch)", i, g, w)
		}

		lead, ok := g.Lead.Get()
		leadWant, leadWantOK := w.Lead.Get()
		if ok != leadWantOK || (ok && lead != leadWant) {
			t.Fatalf("got[%d].Lead = (%q, %v), want (%q, %v)", i, lead, ok, leadWant, leadWantOK)
		}

		lag, ok := g.Lag.Get()
		lagWant, lagWantOK := w.Lag.Get()
		if ok != lagWantOK || (ok && lag != lagWant) {
			t.Fatalf("got[%d].Lag = (%q, %v), want (%q, %v)", i, lag, ok, lagWant, lagWantOK)
		}
	}
}

// TestWindowQueryPartitionBy proves PARTITION BY partitions the window per
// category: the row_number resets to 1 at each partition boundary instead
// of running across the whole result. WindowQuery projects only the
// entity's own columns (category is an extra table column, not scannable
// here), so the test proves the reset through the qty/rn pairs alone.
func TestWindowQueryPartitionBy(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	type row struct {
		qty int64
		rn  int64
	}

	var got []row

	err := From(widgets).
		Select(RowNumber[widget]().Over([]AnyColumn[widget]{widgetCategory.Col()}, []OrderTerm[widget]{widgetQty.Asc()})).
		OrderBy(widgetQty.Asc()).
		Scan(ctx, conn, func(r Row) error {
			var (
				id, name string
				qty      int64
				bio      Option[string]
				rn       int64
			)

			if err := r.Scan(&id, &name, &qty, &bio, &rn); err != nil {
				return err
			}

			got = append(got, row{qty: qty, rn: rn})

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Ordered by quantity ascending: tool's single row (qty 5, its own
	// partition -> rn 1), then fruit's rows (qty 10/20/30 -> rn 1/2/3).
	// Without the partition, rn would run 1,2,3,4 across the whole result.
	want := []row{
		{qty: 5, rn: 1},
		{qty: 10, rn: 1},
		{qty: 20, rn: 2},
		{qty: 30, rn: 3},
	}

	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// TestWindowQueryWhereAndLimit proves WindowQuery composes Where/Limit
// with the window expressions.
func TestWindowQueryWhereAndLimit(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var rns []int64

	err := From(widgets).
		Where(widgetQty.Gte(20)).
		Select(RowNumber[widget]().Over(nil, []OrderTerm[widget]{widgetQty.Asc()})).
		OrderBy(widgetQty.Asc()).
		Limit(1).
		Scan(ctx, conn, func(r Row) error {
			var (
				id, name string
				qty      int64
				bio      Option[string]
				rn       int64
			)

			if err := r.Scan(&id, &name, &qty, &bio, &rn); err != nil {
				return err
			}

			rns = append(rns, rn)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// WHERE excludes w1; ROW_NUMBER is computed over the remaining rows
	// (w2, w3) before LIMIT 1 trims to the first.
	if len(rns) != 1 || rns[0] != 1 {
		t.Fatalf("rns = %v, want [1]", rns)
	}
}

// TestWindowFrameRoundTrip proves ROWS, RANGE and GROUPS frame clauses
// against real SQLite: a ROWS running total, a ROWS moving window, a RANGE
// unbounded-preceding frame, and a GROUPS current-row-to-unbounded-following
// reverse running total. Each expression is scanned positionally after the
// entity's own columns.
func TestWindowFrameRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	type frameRow struct {
		qty                     int64
		running, moving, ranged int64
		groups                  int64
	}

	var got []frameRow

	err := From(widgets).
		Select(
			SumOver[widget](widgetQty).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}).RowsBetween(UnboundedPreceding(), CurrentRow()),
			SumOver[widget](widgetQty).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}).RowsBetween(Preceding(1), Following(1)),
			SumOver[widget](widgetQty).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}).RangeBetween(UnboundedPreceding(), CurrentRow()),
			SumOver[widget](widgetQty).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}).GroupsBetween(CurrentRow(), UnboundedFollowing()),
		).
		OrderBy(widgetQty.Asc()).
		Scan(ctx, conn, func(r Row) error {
			var (
				id, name string
				qty      int64
				bio      Option[string]
				row      frameRow
			)

			if err := r.Scan(&id, &name, &qty, &bio, &row.running, &row.moving, &row.ranged, &row.groups); err != nil {
				return err
			}

			row.qty = qty

			got = append(got, row)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Rows ordered by quantity ascending: w1(10), w2(20), w3(30), total 60.
	want := []frameRow{
		{qty: 10, running: 10, moving: 30, ranged: 10, groups: 60},
		{qty: 20, running: 30, moving: 60, ranged: 30, groups: 50},
		{qty: 30, running: 60, moving: 50, ranged: 60, groups: 30},
	}

	if len(got) != len(want) {
		t.Fatalf("Scan returned %d rows, want %d", len(got), len(want))
	}

	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// TestWindowFrameScanCapabilityGate drives the frame capability gate through
// the public WindowQuery.Scan path: GROUPS on sqlite pinned to 3.27 (which
// predates frames) and any frame on a base-only dialect return the typed
// dialect.ErrUnsupportedByDialect, while default sqlite ROWS passes (the
// mock's empty result set yields no error).
func TestWindowFrameScanCapabilityGate(t *testing.T) {
	ctx := context.Background()

	groups := SumOver[widget](widgetQty).Over(nil, []OrderTerm[widget]{widgetQty.Asc()}).GroupsBetween(CurrentRow(), UnboundedFollowing())
	rows := SumOver[widget](widgetQty).Over(nil, nil).RowsBetween(UnboundedPreceding(), CurrentRow())

	if err := From(widgets).Select(groups).Scan(ctx, mockExec{dialectName: "sqlite-3.27"}, func(Row) error { return nil }); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("sqlite-3.27 GROUPS err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if err := From(widgets).Select(rows).Scan(ctx, mockExec{dialectName: "mock-nocap"}, func(Row) error { return nil }); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("base-only ROWS err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if err := From(widgets).Select(rows).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil }); err != nil {
		t.Fatalf("sqlite ROWS err = %v, want nil (SQLite 3.28+ supports ROWS frames)", err)
	}
}

// TestWindowQueryScanErrorPaths drives resolve, query, row-callback,
// iteration and close failures through WindowQuery.Scan.
func TestWindowQueryScanErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	q := From(widgets).Select(RowNumber[widget]().Over(nil, nil))

	if err := q.Scan(ctx, fakeDB{}, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan on an unresolvable dialect succeeded, want an error")
	}

	queryErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	if err := q.Scan(ctx, queryErr, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	fnErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"w1"}}}}

	if err := q.Scan(ctx, fnErr, func(Row) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"w1"}}, iterErr: boom}}

	if err := q.Scan(ctx, iterErr, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{closeErr: boom}}

	if err := q.Scan(ctx, closeErr, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}
}
