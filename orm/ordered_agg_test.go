package orm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
	"github.com/zenta-dev/zever/orm/render"
)

func init() {
	// sqlite-3.43 is the pre-3.44 floor: in-aggregate ORDER BY arrived in
	// SQLite 3.44.0, so the version-gate tests can prove group_concat still
	// renders unordered there while an ordered form is a typed error.
	old, err := sqlite.NewWithVersion("3.43.0")
	if err != nil {
		panic(err) //nolint:forbidigo // test-only literal; the version cannot fail to parse
	}

	if err := dialect.Register("sqlite-3.43", func() dialect.Dialect { return old }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration; test files are exempt but the guard stays explicit
	}
}

// TestOrderedAggregateConstructors proves the typed constructors erase to the
// expected Aggregate and that OrderBy records identifier order terms without
// mutating the receiver.
func TestOrderedAggregateConstructors(t *testing.T) {
	arr := ArrayAgg(widgetQty)
	if arr.Func != AggArrayAgg || arr.Column != "quantity" || arr.Alias != "array_agg_quantity" {
		t.Fatalf("ArrayAgg(widgetQty) = %+v, want array_agg over quantity", arr.Aggregate)
	}

	ordered := arr.OrderBy(widgetName.Asc())
	if ordered.Func != AggArrayAgg || ordered.ordered == nil || len(ordered.ordered.terms) != 1 {
		t.Fatalf("ArrayAgg(...).OrderBy(...) = %+v, want one order term", ordered)
	}

	if ordered.ordered.terms[0].column != "name" || ordered.ordered.terms[0].desc {
		t.Fatalf("order term = %+v, want name ASC", ordered.ordered.terms[0])
	}

	if arr.ordered != nil {
		t.Fatal("OrderBy mutated its receiver")
	}

	str := StringAgg(widgetName, ", ")
	if str.Func != AggStringAgg || !str.hasDelim || str.delim != ", " {
		t.Fatalf("StringAgg = %+v, want string_agg with bound delimiter", str.Aggregate)
	}

	group := GroupConcat(widgetName, ";")
	if group.Func != AggGroupConcat || !group.hasDelim || group.delim != ";" {
		t.Fatalf("GroupConcat = %+v, want group_concat with delimiter", group.Aggregate)
	}

	noDelim := GroupConcat(widgetName)
	if noDelim.hasDelim || noDelim.bad != "" {
		t.Fatalf("GroupConcat() = %+v, want no delimiter and valid", noDelim.Aggregate)
	}
}

// TestOrderedAggregateSQLiteRoundTrip proves group_concat(... ORDER BY ...)
// renders and executes against real SQLite, concatenating each group's names
// in quantity order.
func TestOrderedAggregateSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	got := map[string]string{}

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(GroupConcat(widgetName, ",").OrderBy(widgetQty.Asc())).
		Scan(ctx, conn, func(r Row) error {
			var (
				cat string
				s   string
			)

			if err := r.Scan(&cat, &s); err != nil {
				return err
			}

			got[cat] = s

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got["fruit"] != "Apple,Banana,Cherry" {
		t.Fatalf("fruit = %q, want Apple,Banana,Cherry (ordered by quantity)", got["fruit"])
	}

	if got["tool"] != "Drill" {
		t.Fatalf("tool = %q, want Drill", got["tool"])
	}
}

// TestOrderedAggregatePostgresSyntax pins the exact Postgres rendering: the
// delimiter is a bound placeholder and the order term is an identifier.
func TestOrderedAggregatePostgresSyntax(t *testing.T) {
	rec := &ormRecordingExec{dialectName: "postgres"}
	ctx := t.Context()

	err := From(widgets).
		GroupBy().
		Agg(StringAgg(widgetName, ", ").OrderBy(widgetQty.Asc())).
		Scan(ctx, rec, func(Row) error { return nil })
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	q, args := rec.last()

	want := `SELECT string_agg("name", $1 ORDER BY "quantity" ASC) AS "string_agg_name" FROM "widgets"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 1 || args[0] != ", " {
		t.Fatalf("args = %#v, want [\", \"]", args)
	}
}

// TestOrderedAggregateDialectGates proves each constructor is rejected on the
// dialects that lack its function with the typed
// dialect.ErrUnsupportedByDialect.
func TestOrderedAggregateDialectGates(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name string
		dial string
		run  func(exec db.DB) error
	}{
		{"array_agg/sqlite", "sqlite", func(exec db.DB) error {
			return From(widgets).GroupBy().Agg(ArrayAgg(widgetQty)).Scan(ctx, exec, func(Row) error { return nil })
		}},
		{"string_agg/sqlite", "sqlite", func(exec db.DB) error {
			return From(widgets).GroupBy().Agg(StringAgg(widgetName, ",").OrderBy(widgetQty.Asc())).Scan(ctx, exec, func(Row) error { return nil })
		}},
		{"group_concat/postgres", "postgres", func(exec db.DB) error {
			return From(widgets).GroupBy().Agg(GroupConcat(widgetName, ",").OrderBy(widgetQty.Asc())).Scan(ctx, exec, func(Row) error { return nil })
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run(mockExec{dialectName: tc.dial})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestOrderedAggregateSQLiteVersionGate pins the SQLite 3.44.0 floor: an
// unordered group_concat still renders on 3.43, while an ordered form is a
// typed error there.
func TestOrderedAggregateSQLiteVersionGate(t *testing.T) {
	ctx := t.Context()

	if err := From(widgets).GroupBy().Agg(GroupConcat(widgetName, ",")).Scan(ctx, mockExec{dialectName: "sqlite-3.43"}, func(Row) error { return nil }); err != nil {
		t.Fatalf("unordered group_concat on 3.43 err = %v, want nil", err)
	}

	err := From(widgets).GroupBy().Agg(GroupConcat(widgetName, ",").OrderBy(widgetQty.Asc())).Scan(ctx, mockExec{dialectName: "sqlite-3.43"}, func(Row) error { return nil })
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("ordered group_concat on 3.43 err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestOrderedAggregateBadOrderTermFailsClosed proves an ORDER BY term that is
// not a plain column (an expression, here) is a typed rendering error, never
// silently dropped.
func TestOrderedAggregateBadOrderTermFailsClosed(t *testing.T) {
	ctx := t.Context()

	err := From(widgets).
		GroupBy().
		Agg(ArrayAgg(widgetQty).OrderBy(Lower(widgetName.Expr()).Asc())).
		Scan(ctx, mockExec{dialectName: "postgres"}, func(Row) error { return nil })
	if !errors.Is(err, render.ErrUnsupported) {
		t.Fatalf("err = %v, want errors.Is(err, render.ErrUnsupported)", err)
	}
}

// TestGroupConcatTooManyDelimitersFailsClosed proves the variadic delimiter
// rejects more than one value rather than silently picking one.
func TestGroupConcatTooManyDelimitersFailsClosed(t *testing.T) {
	ctx := t.Context()

	err := From(widgets).
		GroupBy().
		Agg(GroupConcat(widgetName, ",", ";")).
		Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil })
	if !errors.Is(err, render.ErrUnsupported) {
		t.Fatalf("err = %v, want errors.Is(err, render.ErrUnsupported)", err)
	}
}

// TestOrderedAggregateCopyOnWrite proves Agg does not mutate the base
// GroupedQuery's aggregate slice.
func TestOrderedAggregateCopyOnWrite(t *testing.T) {
	base := From(widgets).GroupBy(widgetCategory.Col())

	branch := base.Agg(GroupConcat(widgetName, ",").OrderBy(widgetQty.Asc()))

	if len(base.aggs) != 0 {
		t.Fatalf("base.aggs mutated: %v", base.aggs)
	}

	if len(branch.aggs) != 1 || branch.aggs[0].Func != AggGroupConcat {
		t.Fatalf("branch.aggs = %v, want one group_concat", branch.aggs)
	}
}
