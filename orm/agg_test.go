package orm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/render"
)

// TestAggregateConstructors proves Count/Sum/Avg/Min/Max and their
// *Nullable variants build the expected erased Aggregate value.
func TestAggregateConstructors(t *testing.T) {
	if got, want := Count(), (Aggregate{Func: AggCount, Alias: "count"}); got != want {
		t.Fatalf("Count() = %+v, want %+v", got, want)
	}

	if got, want := Sum[int64](widgetQty), (Aggregate{Func: AggSum, Column: "quantity", Alias: "sum_quantity"}); got != want {
		t.Fatalf("Sum(widgetQty) = %+v, want %+v", got, want)
	}

	if got, want := Avg[int64](widgetQty), (Aggregate{Func: AggAvg, Column: "quantity", Alias: "avg_quantity"}); got != want {
		t.Fatalf("Avg(widgetQty) = %+v, want %+v", got, want)
	}

	if got, want := Min[string](widgetName), (Aggregate{Func: AggMin, Column: "name", Alias: "min_name"}); got != want {
		t.Fatalf("Min(widgetName) = %+v, want %+v", got, want)
	}

	if got, want := Max[string](widgetName), (Aggregate{Func: AggMax, Column: "name", Alias: "max_name"}); got != want {
		t.Fatalf("Max(widgetName) = %+v, want %+v", got, want)
	}

	if got, want := SumNullable[int64](widgetQtyNullable), (Aggregate{Func: AggSum, Column: "bio_len", Alias: "sum_bio_len"}); got != want {
		t.Fatalf("SumNullable(widgetQtyNullable) = %+v, want %+v", got, want)
	}

	if got, want := AvgNullable[int64](widgetQtyNullable), (Aggregate{Func: AggAvg, Column: "bio_len", Alias: "avg_bio_len"}); got != want {
		t.Fatalf("AvgNullable(widgetQtyNullable) = %+v, want %+v", got, want)
	}

	if got, want := MinNullable[string](widgetBio), (Aggregate{Func: AggMin, Column: "bio", Alias: "min_bio"}); got != want {
		t.Fatalf("MinNullable(widgetBio) = %+v, want %+v", got, want)
	}

	if got, want := MaxNullable[string](widgetBio), (Aggregate{Func: AggMax, Column: "bio", Alias: "max_bio"}); got != want {
		t.Fatalf("MaxNullable(widgetBio) = %+v, want %+v", got, want)
	}
}

// widgetQtyNullable is a NullableColumn fixture purely for
// SumNullable/AvgNullable's construction test above -- widgets.bio_len
// doesn't need to exist as a real table column for a pure builder test.
var widgetQtyNullable = NewNullableColumn[widget, int64]("widgets", "bio_len")

// TestGroupByAggHavingDoesNotMutateBase proves GroupBy/Agg/Having follow
// the same copy-on-write discipline as Query[T,PT]/Update[T]/Insert[T].
func TestGroupByAggHavingDoesNotMutateBase(t *testing.T) {
	base := From(widgets).GroupBy(widgetName.Col())

	branchA := base.Agg(Count())
	branchB := base.Agg(Sum[int64](widgetQty))

	if len(base.aggs) != 0 {
		t.Fatalf("base.aggs mutated by branching: %v", base.aggs)
	}

	if len(branchA.aggs) != 1 || branchA.aggs[0].Func != AggCount {
		t.Fatalf("branchA.aggs = %v, want one COUNT aggregate", branchA.aggs)
	}

	if len(branchB.aggs) != 1 || branchB.aggs[0].Func != AggSum {
		t.Fatalf("branchB.aggs = %v, want one SUM aggregate", branchB.aggs)
	}

	baseHaving := base.Having(Count().Gt(1))
	if base.having.IsSet() {
		t.Fatalf("base.having.IsSet() = true after Having, want base left unmodified")
	}

	if !baseHaving.having.IsSet() {
		t.Fatalf("baseHaving.having.IsSet() = false, want true")
	}
}

// TestGroupByCarriesQueryWhere proves GroupBy keeps the originating
// Query[T,PT]'s WHERE filter.
func TestGroupByCarriesQueryWhere(t *testing.T) {
	g := From(widgets).Where(widgetQty.Gt(0)).GroupBy(widgetName.Col())

	if !g.where.IsSet() {
		t.Fatalf("g.where.IsSet() = false, want the originating Query's Where to carry over")
	}
}

// TestGroupedQueryScanRoundTrip proves GroupBy(...).Agg(...) against real
// SQLite: seed several widgets sharing a couple of "categories" (encoded
// via Name's first letter for this fixture), aggregate, and check the
// returned rows match hand-computed expectations.
func TestGroupedQueryScanRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	type row struct {
		category string
		count    int64
		total    int64
	}

	var got []row

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(Count(), Sum[int64](widgetQty)).
		Having(Count().Gt(1)).
		Scan(ctx, conn, func(r Row) error {
			var rw row
			if err := r.Scan(&rw.category, &rw.count, &rw.total); err != nil {
				return err
			}

			got = append(got, rw)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Fixture: category "fruit" has 3 rows (qty 10,20,30 -> count=3,
	// sum=60); category "tool" has 1 row (qty 5) -- HAVING COUNT(*) > 1
	// must filter "tool" out entirely. This is the critical proof HAVING
	// isn't a no-op.
	if len(got) != 1 {
		t.Fatalf("got %d groups, want exactly 1 (having filtered out the single-row group): %+v", len(got), got)
	}

	if got[0].category != "fruit" || got[0].count != 3 || got[0].total != 60 {
		t.Fatalf("got[0] = %+v, want {fruit 3 60}", got[0])
	}
}

// TestGroupedQueryScanWithoutHaving proves the same fixture with no HAVING
// clause returns both groups, confirming HAVING (not GROUP BY itself) is
// what filtered the previous test's "tool" group out.
func TestGroupedQueryScanWithoutHaving(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	var categories []string

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(Count()).
		Scan(ctx, conn, func(r Row) error {
			var (
				cat string
				n   int64
			)

			if err := r.Scan(&cat, &n); err != nil {
				return err
			}

			categories = append(categories, cat)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(categories) != 2 {
		t.Fatalf("got %d groups, want 2 (no HAVING filter applied): %v", len(categories), categories)
	}
}

// TestGroupedQueryWhereAndHavingArgOrder proves a grouped query combining
// both Where (on the underlying Query[T,PT]) and Having produces args in
// [where-args..., having-args...] order, matching the rendered SQL's
// placeholder order -- exercised against real SQLite so a wrong arg order
// would surface as a wrong/empty result set, not just a string mismatch.
func TestGroupedQueryWhereAndHavingArgOrder(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	var got []string

	err := From(widgets).
		Where(widgetQty.Gt(int64(6))). // excludes the qty=5 "tool" row
		GroupBy(widgetCategory.Col()).
		Agg(Sum[int64](widgetQty)).
		Having(Sum[int64](widgetQty).Gte(int64(50))). // only "fruit" (sum=60) qualifies
		Scan(ctx, conn, func(r Row) error {
			var (
				cat string
				sum int64
			)

			if err := r.Scan(&cat, &sum); err != nil {
				return err
			}

			got = append(got, cat)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != 1 || got[0] != "fruit" {
		t.Fatalf("got = %v, want [\"fruit\"] (WHERE excludes tool, HAVING excludes anything under 50)", got)
	}
}

// widgetCategory is a fixture NullableColumn-free plain Column over a
// "category" column added by newCategorizedWidgetsDB below.
var widgetCategory = NewColumn[widget, string]("widgets", "category")

// newCategorizedWidgetsDB seeds an in-memory SQLite widgets table (same
// shape newWidgetsDB uses, plus a "category" column) with rows spanning two
// groups: "fruit" (3 rows, qty 10/20/30) and "tool" (1 row, qty 5).
func newCategorizedWidgetsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text, category text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct {
		id, name, category string
		qty                int64
	}{
		{"w1", "Apple", "fruit", 10},
		{"w2", "Banana", "fruit", 20},
		{"w3", "Cherry", "fruit", 30},
		{"w4", "Drill", "tool", 5},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio, category) VALUES (?, ?, ?, NULL, ?)`, r.id, r.name, r.qty, r.category); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}

// TestHavingComparisons proves every aggregate comparison renders and
// filters: each op selects the expected group(s) against real SQLite.
func TestHavingComparisons(t *testing.T) {
	for _, tc := range []struct {
		name string
		hp   HavingPredicate
		want []string
	}{
		{"eq", Count().Eq(3), []string{"fruit"}},
		{"neq", Count().Neq(3), []string{"tool"}},
		{"gt", Count().Gt(1), []string{"fruit"}},
		{"gte", Count().Gte(3), []string{"fruit"}},
		{"lt", Count().Lt(3), []string{"tool"}},
		{"lte", Count().Lte(1), []string{"tool"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn := newCategorizedWidgetsDB(t)

			var got []string

			err := From(widgets).
				GroupBy(widgetCategory.Col()).
				Agg(Count()).
				Having(tc.hp).
				Scan(ctx, conn, func(r Row) error {
					var cat string

					var n int64

					if err := r.Scan(&cat, &n); err != nil {
						return err
					}

					got = append(got, cat)

					return nil
				})
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}

			if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Fatalf("groups = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHavingCombinators proves HavingOr/HavingNot compose, collapse and
// filter: OR across two comparisons keeps both groups, NOT inverts, and
// empty combinators leave the query unfiltered.
func TestHavingCombinators(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	scanCats := func(q GroupedQuery[widget]) []string {
		t.Helper()

		var got []string

		err := q.Scan(ctx, conn, func(r Row) error {
			var cat string

			var n int64

			if err := r.Scan(&cat, &n); err != nil {
				return err
			}

			got = append(got, cat)

			return nil
		})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}

		return got
	}

	base := From(widgets).GroupBy(widgetCategory.Col()).Agg(Count())

	if got := scanCats(base.Having(HavingOr(Count().Eq(3), Count().Eq(1)))); len(got) != 2 {
		t.Fatalf("or groups = %v, want both groups", got)
	}

	if got := scanCats(base.Having(HavingNot(Count().Eq(3)))); len(got) != 1 || got[0] != "tool" {
		t.Fatalf("not groups = %v, want [tool]", got)
	}

	if got := scanCats(base.Having(HavingOr())); len(got) != 2 {
		t.Fatalf("empty-or groups = %v, want both groups (unset filter)", got)
	}

	if got := scanCats(base.Having(HavingNot(HavingPredicate{}))); len(got) != 2 {
		t.Fatalf("not-unset groups = %v, want both groups (unset filter)", got)
	}

	if got := scanCats(base.Having(HavingAnd(Count().Gt(1)))); len(got) != 1 || got[0] != "fruit" {
		t.Fatalf("single-and groups = %v, want [fruit]", got)
	}

	if got := scanCats(base.Having(widgetCategory.Eq("fruit")) /* scalar-expr HAVING */); len(got) != 1 || got[0] != "fruit" {
		t.Fatalf("scalar groups = %v, want [fruit]", got)
	}
}

// TestGroupedQueryScanErrorPaths drives resolve, query, row-callback,
// iteration and close failures through GroupedQuery.Scan.
func TestGroupedQueryScanErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	q := From(widgets).GroupBy(widgetCategory.Col()).Agg(Count())

	if err := q.Scan(ctx, fakeDB{}, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan on an unresolvable dialect succeeded, want an error")
	}

	bad := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	if err := q.Scan(ctx, bad, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	fnErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"fruit", int64(3)}}}}

	if err := q.Scan(ctx, fnErr, func(Row) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"fruit", int64(3)}}, iterErr: boom}}

	if err := q.Scan(ctx, iterErr, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{closeErr: boom}}

	if err := q.Scan(ctx, closeErr, func(Row) error { return nil }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}
}

// TestOrderedAggregateOrderByRejectsBadTerms proves the ORDER BY validator:
// a preset-bad aggregate passes through, an empty column and an expression
// term fail closed at render time.
func TestOrderedAggregateOrderByRejectsBadTerms(t *testing.T) {
	ctx := t.Context()

	bad := GroupConcat(widgetName, ",", ";").OrderBy(widgetQty.Asc())
	if bad.bad == "" {
		t.Fatal("expected a preset-bad aggregate to stay bad through OrderBy")
	}

	if err := From(widgets).GroupBy().Agg(bad).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan with a preset-bad ordered aggregate succeeded, want an error")
	}

	empty := ArrayAgg(widgetQty).OrderBy(OrderTerm[widget]{})
	if empty.bad == "" {
		t.Fatal("expected an empty-column ORDER BY term to fail closed")
	}

	if err := From(widgets).GroupBy().Agg(empty).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan with an empty-column ORDER BY term succeeded, want an error")
	}
}

// TestGroupByBareColumnExprRoundTrip proves a bare-column expression GROUP
// BY (the NColumn leaf) groups identically to the plain column form.
func TestGroupByBareColumnExprRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	groups := 0

	err := From(widgets).
		GroupBy(widgetName.Expr()).
		Agg(Count()).
		Scan(ctx, conn, func(Row) error {
			groups++

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if groups != 3 {
		t.Fatalf("groups = %d, want 3", groups)
	}
}

// TestToRenderHavingUnknownKind proves an out-of-range HAVING node kind
// erases to the zero node rather than panicking.
func TestToRenderHavingUnknownKind(t *testing.T) {
	if got := toRenderHaving(havingNode{kind: havingKind(99)}); !reflect.DeepEqual(got, render.HavingNode{}) {
		t.Fatalf("toRenderHaving(99) = %+v, want zero", got)
	}
}
