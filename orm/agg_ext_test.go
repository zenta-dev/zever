package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestAggregateExtensionCapabilityCompile guards the per-dialect capability
// interface against signature drift: every supported dialect must satisfy
// dialect.AggregateGroupingDialect.
func TestAggregateExtensionCapabilityCompile(_ *testing.T) {
	var (
		_ dialect.AggregateGroupingDialect = postgres.New()
		_ dialect.AggregateGroupingDialect = sqlite.New()
	)
}

// TestAggregateGroupingCapabilityTruthTable pins the FILTER / ROLLUP / CUBE /
// GROUPING SETS capability matrix: Postgres supports all four, SQLite only
// FILTER (ROLLUP/CUBE/GROUPING SETS are typed errors).
func TestAggregateGroupingCapabilityTruthTable(t *testing.T) {
	p := postgres.New()
	if !p.SupportsAggregateFilter() || !p.SupportsRollup() || !p.SupportsCube() || !p.SupportsGroupingSets() {
		t.Fatalf("postgres capabilities = (%v, %v, %v, %v), want all true",
			p.SupportsAggregateFilter(), p.SupportsRollup(), p.SupportsCube(), p.SupportsGroupingSets())
	}

	s := sqlite.New()
	if !s.SupportsAggregateFilter() {
		t.Fatal("sqlite.SupportsAggregateFilter() = false, want true")
	}

	if s.SupportsRollup() || s.SupportsCube() || s.SupportsGroupingSets() {
		t.Fatalf("sqlite grouping-set capabilities = (%v, %v, %v), want all false",
			s.SupportsRollup(), s.SupportsCube(), s.SupportsGroupingSets())
	}
}

// TestAggregateExtensionsConstructors proves Filter/Distinct/CountDistinct
// build the expected erased Aggregate values without mutating the receiver.
func TestAggregateExtensionsConstructors(t *testing.T) {
	base := Count()
	if base.distinct || base.filter != nil {
		t.Fatalf("Count() carries distinct/filter state: %+v", base)
	}

	distinct := base.Distinct()
	if !distinct.distinct {
		t.Fatal("Distinct() did not set the distinct flag")
	}

	if base.distinct {
		t.Fatal("Distinct() mutated its receiver")
	}

	pred := widgetQty.Gt(int64(5))
	filtered := base.Filter(pred)
	if filtered.filter == nil {
		t.Fatal("Filter() did not attach the predicate")
	}

	if base.filter != nil {
		t.Fatal("Filter() mutated its receiver")
	}

	cd := CountDistinct(widgetName.Col())
	if cd.Func != AggCount || cd.Column != "name" || !cd.distinct {
		t.Fatalf("CountDistinct(widgetName) = %+v, want COUNT(DISTINCT name)", cd)
	}
}

// TestAggregateFilterSQLiteRoundTrip proves FILTER (WHERE ...) renders and
// executes against real SQLite: the unfiltered COUNT counts every row while
// the filtered COUNT counts only rows whose quantity exceeds 5.
func TestAggregateFilterSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	type row struct {
		category string
		all      int64
		overFive int64
	}

	got := map[string]row{}

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(Count(), Count().Filter(widgetQty.Gt(int64(5)))).
		Scan(ctx, conn, func(r Row) error {
			var rw row
			if err := r.Scan(&rw.category, &rw.all, &rw.overFive); err != nil {
				return err
			}

			got[rw.category] = rw

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got["fruit"] != (row{category: "fruit", all: 3, overFive: 3}) {
		t.Fatalf("fruit = %+v, want all=3 overFive=3", got["fruit"])
	}

	if got["tool"] != (row{category: "tool", all: 1, overFive: 0}) {
		t.Fatalf("tool = %+v, want all=1 overFive=0", got["tool"])
	}
}

// TestCountDistinctSQLiteRoundTrip proves COUNT(DISTINCT col) renders and
// executes against real SQLite: "fruit" has three distinct names, "tool" one.
func TestCountDistinctSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	got := map[string]int64{}

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(CountDistinct(widgetName.Col())).
		Scan(ctx, conn, func(r Row) error {
			var (
				cat string
				n   int64
			)

			if err := r.Scan(&cat, &n); err != nil {
				return err
			}

			got[cat] = n

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got["fruit"] != 3 || got["tool"] != 1 {
		t.Fatalf("got = %v, want fruit=3 tool=1", got)
	}
}

// TestGroupByExpressionSQLiteRoundTrip proves GroupBy accepts a scalar
// expression: grouping by LOWER(name) yields one group per lowercased name.
func TestGroupByExpressionSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	var groups int

	err := From(widgets).
		GroupBy(Lower(widgetName.Expr())).
		Agg(Count()).
		Scan(ctx, conn, func(Row) error {
			groups++

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if groups != 4 {
		t.Fatalf("groups = %d, want 4 (Apple/Banana/Cherry/Drill lowercased)", groups)
	}
}

// TestHavingExpressionSQLiteRoundTrip proves Having accepts a scalar
// expression predicate: HAVING LOWER(category) = 'fruit' keeps only the
// fruit group.
func TestHavingExpressionSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	var got []string

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(Count()).
		Having(Lower(widgetCategory.Expr()).Eq("fruit")).
		Scan(ctx, conn, func(r Row) error {
			var cat string
			if err := r.Scan(&cat, new(int64)); err != nil {
				return err
			}

			got = append(got, cat)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != 1 || got[0] != "fruit" {
		t.Fatalf("got = %v, want [fruit]", got)
	}
}

// TestHavingAggregateAndExpressionCombine proves an aggregate HavingPredicate
// and a scalar expression predicate compose with AND.
func TestHavingAggregateAndExpressionCombine(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	var got []string

	err := From(widgets).
		GroupBy(widgetCategory.Col()).
		Agg(Count()).
		Having(Count().Gt(1), Lower(widgetCategory.Expr()).Eq("fruit")).
		Scan(ctx, conn, func(r Row) error {
			var cat string
			if err := r.Scan(&cat, new(int64)); err != nil {
				return err
			}

			got = append(got, cat)

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(got) != 1 || got[0] != "fruit" {
		t.Fatalf("got = %v, want [fruit] (AND of COUNT>1 and LOWER(category)=fruit)", got)
	}
}

// TestGroupingConstructsSQLiteGate proves ROLLUP/CUBE/GROUPING SETS are
// rejected on SQLite with the typed dialect.ErrUnsupportedByDialect.
func TestGroupingConstructsSQLiteGate(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"Rollup", func() error {
			return From(widgets).GroupBy(Rollup(widgetCategory.Col(), widgetName.Col())).Agg(Count()).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil })
		}},
		{"Cube", func() error {
			return From(widgets).GroupBy(Cube(widgetCategory.Col(), widgetName.Col())).Agg(Count()).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil })
		}},
		{"GroupingSets", func() error {
			return From(widgets).GroupBy(GroupingSets(NewGroupingSet(widgetCategory.Col()), NewGroupingSet(widgetName.Col()))).Agg(Count()).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestGroupByExpressionArgSQLiteRoundTrip proves an expression GROUP BY
// whose expression binds an argument still executes against real SQLite (the
// argument is bound once in the select list and once in GROUP BY).
func TestGroupByExpressionArgSQLiteRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var groups int

	err := From(widgets).
		GroupBy(CoalesceNullable(widgetBio, "n/a")).
		Agg(Count()).
		Scan(ctx, conn, func(Row) error {
			groups++

			return nil
		})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if groups != 3 {
		t.Fatalf("groups = %d, want 3 (first, n/a, third)", groups)
	}
}

// TestWithGroupedRejectsAdvancedGroups proves wrapping a GroupedQuery that
// uses an expression or grouping-set GROUP BY in a CTE is a typed error
// rather than a silently-dropped grouping.
func TestWithGroupedRejectsAdvancedGroups(t *testing.T) {
	ctx := context.Background()

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := From(widgets).GroupBy(Lower(widgetName.Expr())).Agg(Count())

	scanErr := WithGrouped(name, inner).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil })
	if scanErr == nil {
		t.Fatal("CTEGroupedQuery.Scan err = nil, want a typed error for an advanced GROUP BY")
	}
}
