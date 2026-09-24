package orm

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// mockDialect implements ONLY the base dialect.Dialect interface -- no
// capability sub-interface -- so every capability type-assertion fails on
// it, exercising the "typed ErrUnsupportedByDialect, never a
// panic or a silent wrong-SQL fallback" path end to end through the public
// All/Count API.
type mockDialect struct{}

func (mockDialect) Name() string               { return "mock-nocap" }
func (mockDialect) Placeholder(int) string     { return "?" }
func (mockDialect) QuoteIdent(s string) string { return `"` + s + `"` }

// noRecursiveDialect implements dialect.CTEDialect (so plain WITH works on
// it) but reports recursive CTEs unsupported, exercising the
// WITH RECURSIVE-specific gate in requireCTE.
type noRecursiveDialect struct{ mockDialect }

func (noRecursiveDialect) SupportsCTE() bool       { return true }
func (noRecursiveDialect) SupportsRecursive() bool { return false }

func init() {
	if err := dialect.Register("mock-nocap", func() dialect.Dialect { return mockDialect{} }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration; test files are exempt but the guard stays explicit
	}

	if err := dialect.Register("mock-norec", func() dialect.Dialect { return noRecursiveDialect{} }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration; test files are exempt but the guard stays explicit
	}

	// sqlite-3.38 is the pre-RIGHT/FULL SQLite floor: registered test-only so
	// the capability-gate tests can prove the same default sqlite dialect
	// that now supports RIGHT/FULL still rejects both when constructed for a
	// library older than 3.39.0.
	sqliteOld, err := sqlite.NewWithVersion("3.38.0")
	if err != nil {
		panic(err) //nolint:forbidigo // test-only literal; the version cannot fail to parse
	}

	if err := dialect.Register("sqlite-3.38", func() dialect.Dialect { return sqliteOld }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration; test files are exempt but the guard stays explicit
	}
}

// mockExec is a db.DB that reports a caller-chosen dialect name and always
// returns an empty result set -- enough for capability-gate tests that must
// reach the All/Count path but never expect real rows.
type mockExec struct{ dialectName string }

func (m mockExec) Query(context.Context, string, ...any) (db.Rows, error) { return emptyRows{}, nil }
func (m mockExec) Exec(context.Context, string, ...any) (int64, error)    { return 0, nil }
func (m mockExec) Ping(context.Context) error                             { return nil }
func (m mockExec) Close(context.Context) error                            { return nil }
func (m mockExec) Dialect() string                                        { return m.dialectName }

type emptyRows struct{}

func (emptyRows) Next() bool                 { return false }
func (emptyRows) Scan(...any) error          { return nil }
func (emptyRows) Close() error               { return nil }
func (emptyRows) Err() error                 { return nil }
func (emptyRows) Columns() ([]string, error) { return nil, nil }

// TestCapabilityInterfacesCompile guards the sqlite/postgres dialect types'
// capability methods against signature drift: both must satisfy
// dialect.CTEDialect and dialect.SetOpDialect.
func TestCapabilityInterfacesCompile(t *testing.T) {
	var (
		_ dialect.CTEDialect         = sqlite.New()
		_ dialect.SetOpDialect       = sqlite.New()
		_ dialect.ReturningDialect   = sqlite.New()
		_ dialect.JoinCapabilities   = sqlite.New()
		_ dialect.MutateJoinDialect  = sqlite.New()
		_ dialect.MutateOrderDialect = sqlite.New()
		_ dialect.NullsOrderDialect  = sqlite.New()
		_ dialect.ArrayDialect       = sqlite.New()
		_ dialect.WindowFrameDialect = sqlite.New()
		_ dialect.CTEDialect         = postgres.New()
		_ dialect.SetOpDialect       = postgres.New()
		_ dialect.ReturningDialect   = postgres.New()
		_ dialect.JoinCapabilities   = postgres.New()
		_ dialect.MutateJoinDialect  = postgres.New()
		_ dialect.MutateOrderDialect = postgres.New()
		_ dialect.NullsOrderDialect  = postgres.New()
		_ dialect.ArrayDialect       = postgres.New()
		_ dialect.WindowFrameDialect = postgres.New()

		_ dialect.CTEMaterializationDialect = sqlite.New()
		_ dialect.CTESearchCycleDialect     = sqlite.New()
		_ dialect.CTEMaterializationDialect = postgres.New()
		_ dialect.CTESearchCycleDialect     = postgres.New()

		_ dialect.OrderedAggregateDialect = sqlite.New()
		_ dialect.OrderedAggregateDialect = postgres.New()
	)

	s := sqlite.New()
	if !s.SupportsRecursive() {
		t.Fatal("sqlite.SupportsRecursive() = false, want true")
	}

	if !s.SupportsIntersectExcept() {
		t.Fatal("sqlite.SupportsIntersectExcept() = false, want true")
	}

	if !s.SupportsReturning() {
		t.Fatal("sqlite.SupportsReturning() = false, want true")
	}

	// SQLite supports RIGHT/FULL JOIN from 3.39.0, and New() pins the
	// modernc driver's bundled 3.46.0, so both report true here. The 3.38
	// floor is pinned separately in TestSQLiteRightFullJoinVersionTruthTable.
	if !s.SupportsRightJoin() {
		t.Fatal("sqlite.SupportsRightJoin() = false, want true (SQLite >=3.39)")
	}

	if !s.SupportsFullJoin() {
		t.Fatal("sqlite.SupportsFullJoin() = false, want true (SQLite >=3.39)")
	}

	if !s.SupportsUpdateJoin() {
		t.Fatal("sqlite.SupportsUpdateJoin() = false, want true")
	}

	if s.SupportsDeleteJoin() {
		t.Fatal("sqlite.SupportsDeleteJoin() = true, want false (no DELETE...USING)")
	}

	if s.SupportsLeftMutateJoin() {
		t.Fatal("sqlite.SupportsLeftMutateJoin() = true, want false (UPDATE...FROM is inner only)")
	}

	p := postgres.New()
	if !p.SupportsRecursive() {
		t.Fatal("postgres.SupportsRecursive() = false, want true")
	}

	if !p.SupportsIntersectExcept() {
		t.Fatal("postgres.SupportsIntersectExcept() = false, want true")
	}

	if !p.SupportsReturning() {
		t.Fatal("postgres.SupportsReturning() = false, want true")
	}

	if !p.SupportsRightJoin() {
		t.Fatal("postgres.SupportsRightJoin() = false, want true")
	}

	if !p.SupportsFullJoin() {
		t.Fatal("postgres.SupportsFullJoin() = false, want true")
	}

	if !p.SupportsUpdateJoin() {
		t.Fatal("postgres.SupportsUpdateJoin() = false, want true")
	}

	if !p.SupportsDeleteJoin() {
		t.Fatal("postgres.SupportsDeleteJoin() = false, want true")
	}

	if p.SupportsLeftMutateJoin() {
		t.Fatal("postgres.SupportsLeftMutateJoin() = true, want false (FROM/USING is inner only)")
	}
}

// TestNullsOrderCapabilityTruthTable pins the NULLS FIRST/LAST capability
// matrix: Postgres supports it at every version, SQLite only from 3.30.0
// (the release that added the syntax). The gate
// turns an unsupported request into a typed ErrUnsupportedByDialect rather
// than silently dropping the modifier.
func TestNullsOrderCapabilityTruthTable(t *testing.T) {
	if !sqlite.New().SupportsNullsOrdering() {
		t.Fatal("sqlite.New() SupportsNullsOrdering() = false, want true")
	}

	if !postgres.New().SupportsNullsOrdering() {
		t.Fatal("postgres.New() SupportsNullsOrdering() = false, want true")
	}
}

// TestArrayCapabilityTruthTable pins the array-quantifier capability matrix:
// only Postgres has the ARRAY[...] constructor and ANY/ALL quantifiers.
// SQLite reports false, so Column.EqAny/NeqAny/EqAll/NeqAll return a typed
// ErrUnsupportedByDialect there rather than rendering syntax the engine
// rejects.
func TestArrayCapabilityTruthTable(t *testing.T) {
	if !postgres.New().SupportsArrayPredicates() {
		t.Fatal("postgres.SupportsArrayPredicates() = false, want true")
	}

	if sqlite.New().SupportsArrayPredicates() {
		t.Fatal("sqlite.SupportsArrayPredicates() = true, want false (no ARRAY[...] / ANY / ALL)")
	}
}

// TestWindowFrameCapabilityTruthTable pins the verified window-frame
// capability matrix: Postgres supports ROWS/RANGE/GROUPS at every version;
// SQLite gained all three in 3.28.0 (so 3.27 reports false, 3.28 true).
// The gaps turn into a
// typed dialect.ErrUnsupportedByDialect at render time rather than invalid
// SQL.
func TestWindowFrameCapabilityTruthTable(t *testing.T) {
	s := sqlite.New()
	if !s.SupportsWindowFrameRows() || !s.SupportsWindowFrameRange() || !s.SupportsWindowFrameGroups() {
		t.Fatalf("sqlite.New() window frames = (%v, %v, %v), want all true (3.46.0 bundled)",
			s.SupportsWindowFrameRows(), s.SupportsWindowFrameRange(), s.SupportsWindowFrameGroups())
	}

	old, err := sqlite.NewWithVersion("3.27.2")
	if err != nil {
		t.Fatalf("sqlite.NewWithVersion: %v", err)
	}

	if old.SupportsWindowFrameRows() || old.SupportsWindowFrameRange() || old.SupportsWindowFrameGroups() {
		t.Fatalf("sqlite 3.27 window frames = (%v, %v, %v), want all false (frames arrived in 3.28.0)",
			old.SupportsWindowFrameRows(), old.SupportsWindowFrameRange(), old.SupportsWindowFrameGroups())
	}

	newer, err := sqlite.NewWithVersion("3.28.0")
	if err != nil {
		t.Fatalf("sqlite.NewWithVersion: %v", err)
	}

	if !newer.SupportsWindowFrameRows() || !newer.SupportsWindowFrameRange() || !newer.SupportsWindowFrameGroups() {
		t.Fatalf("sqlite 3.28 window frames = (%v, %v, %v), want all true",
			newer.SupportsWindowFrameRows(), newer.SupportsWindowFrameRange(), newer.SupportsWindowFrameGroups())
	}

	p := postgres.New()
	if !p.SupportsWindowFrameRows() || !p.SupportsWindowFrameRange() || !p.SupportsWindowFrameGroups() {
		t.Fatalf("postgres window frames = (%v, %v, %v), want all true",
			p.SupportsWindowFrameRows(), p.SupportsWindowFrameRange(), p.SupportsWindowFrameGroups())
	}
}

// TestSQLiteRightFullJoinVersionTruthTable pins the SQLite RIGHT/FULL JOIN
// version gate: both keywords became core SQL in 3.39.0, so a dialect built
// for 3.38.0 must reject them while 3.39.0+ (and the default New(), pinned
// at the modernc driver's 3.46.0) accept them. The gate turns an
// unsupported request into a typed ErrUnsupportedByDialect rather than
// sending syntax the older engine cannot execute.
func TestSQLiteRightFullJoinVersionTruthTable(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"3.38.0", false},
		{"3.39.0", true},
		{"3.46.0", true},
	}

	for _, tc := range cases {
		d, err := sqlite.NewWithVersion(tc.version)
		if err != nil {
			t.Fatalf("NewWithVersion(%q): %v", tc.version, err)
		}

		if got := d.SupportsRightJoin(); got != tc.want {
			t.Errorf("sqlite %s SupportsRightJoin() = %v, want %v", tc.version, got, tc.want)
		}

		if got := d.SupportsFullJoin(); got != tc.want {
			t.Errorf("sqlite %s SupportsFullJoin() = %v, want %v", tc.version, got, tc.want)
		}
	}

	if !sqlite.New().SupportsRightJoin() || !sqlite.New().SupportsFullJoin() {
		t.Fatal("sqlite.New() must support RIGHT and FULL JOIN (bundled SQLite 3.46.0)")
	}
}

// TestMutateOrderCapabilityTruthTable pins the empirically verified
// UPDATE/DELETE ORDER BY / LIMIT / OFFSET capability matrix:
//
// - Postgres rejects ORDER BY on UPDATE/DELETE in ALL forms -- plain and
// FROM/USING joined -- with a syntax error (the grammar has no
// ORDER BY/LIMIT/OFFSET for DML).
// - SQLite only supports these when compiled with
// SQLITE_ENABLE_UPDATE_DELETE_LIMIT, which the modernc driver (this
// module's sqlite adapter) does not enable, so every form is a syntax
// error there too.
//
// The gates exist so unsupported combos surface as typed
// dialect.ErrUnsupportedByDialect instead of invalid SQL.
func TestMutateOrderCapabilityTruthTable(t *testing.T) {
	s := sqlite.New()
	if s.SupportsUpdateOrderLimit() {
		t.Fatal("sqlite.SupportsUpdateOrderLimit() = true, want false (modernc lacks SQLITE_ENABLE_UPDATE_DELETE_LIMIT)")
	}

	if s.SupportsDeleteOrderLimit() {
		t.Fatal("sqlite.SupportsDeleteOrderLimit() = true, want false (modernc lacks SQLITE_ENABLE_UPDATE_DELETE_LIMIT)")
	}

	if s.SupportsMutateOffset() {
		t.Fatal("sqlite.SupportsMutateOffset() = true, want false (no ORDER BY/LIMIT on mutations at all)")
	}

	if s.SupportsJoinedMutateOrderLimit() {
		t.Fatal("sqlite.SupportsJoinedMutateOrderLimit() = true, want false (no ORDER BY/LIMIT on mutations at all)")
	}

	p := postgres.New()
	if p.SupportsUpdateOrderLimit() {
		t.Fatal("postgres.SupportsUpdateOrderLimit() = true, want false (PG grammar has no DML ORDER BY/LIMIT)")
	}

	if p.SupportsDeleteOrderLimit() {
		t.Fatal("postgres.SupportsDeleteOrderLimit() = true, want false (PG grammar has no DML ORDER BY/LIMIT)")
	}

	if p.SupportsMutateOffset() {
		t.Fatal("postgres.SupportsMutateOffset() = true, want false (PG grammar has no DML ORDER BY/LIMIT)")
	}

	if p.SupportsJoinedMutateOrderLimit() {
		t.Fatal("postgres.SupportsJoinedMutateOrderLimit() = true, want false (PG grammar has no DML ORDER BY/LIMIT)")
	}
}

// TestCTECapabilityGate drives every CTE entry point through a dialect that
// lacks CTEDialect entirely, and the recursive-specific gate through a
// dialect that has CTE but no recursive support -- asserting the typed
// error, never a panic.
func TestCTECapabilityGate(t *testing.T) {
	ctx := t.Context()

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	q := From(widgets)
	recursiveRel := NewRelation[widget, widget]("parent", "id", CTETable[widget](name))

	tests := []struct {
		name string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"With.All", func(ctx context.Context, e mockExec) error {
			_, runErr := With(name, q).All(ctx, e)

			return runErr
		}},
		{"With.Count", func(ctx context.Context, e mockExec) error {
			_, runErr := With(name, q).Count(ctx, e)

			return runErr
		}},
		{"WithRecursive.All", func(ctx context.Context, e mockExec) error {
			_, runErr := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).All(ctx, e)

			return runErr
		}},
		{"WithJoin.All", func(ctx context.Context, e mockExec) error {
			_, runErr := WithJoin(name, JoinOn(q, NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders), InnerJoin)).All(ctx, e)

			return runErr
		}},
		{"WithGrouped.Scan", func(ctx context.Context, e mockExec) error {
			return WithGrouped(name, From(widgets).GroupBy(widgetName.Col()).Agg(Count())).Scan(ctx, e, func(Row) error { return nil })
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"/no-CTE-dialect", func(t *testing.T) {
			runErr := tc.run(ctx, mockExec{dialectName: "mock-nocap"})
			if !errors.Is(runErr, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", runErr)
			}
		})
	}

	// Plain WITH must still work on a CTE-capable dialect that lacks
	// recursive support; only WITH RECURSIVE is gated further.
	plainErr := (func() error {
		_, innerErr := With(name, q).All(ctx, mockExec{dialectName: "mock-norec"})

		return innerErr
	})()
	if plainErr != nil {
		t.Fatalf("plain With on a no-recursive CTE dialect failed: %v", plainErr)
	}

	err = (func() error {
		_, innerErr := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).All(ctx, mockExec{dialectName: "mock-norec"})

		return innerErr
	})()
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("WithRecursive err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestSetOpCapabilityGate asserts UNION/UNION ALL run on a base-only
// dialect (universal SQL, deliberately ungated), while INTERSECT/EXCEPT
// return the typed ErrUnsupportedByDialect -- never a panic or a silent
// wrong-SQL fallback.
func TestSetOpCapabilityGate(t *testing.T) {
	ctx := t.Context()

	q := From(widgets)

	if _, err := Union(q, q).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("Union on a base-only dialect failed: %v (UNION is universal SQL and must not be gated)", err)
	}

	if _, err := UnionAll(q, q).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("UnionAll on a base-only dialect failed: %v (UNION ALL is universal SQL and must not be gated)", err)
	}

	for _, tc := range []struct {
		name string
		s    SetOpQuery[widget, *widget]
	}{
		{"Intersect", Intersect(q, q)},
		{"Except", Except(q, q)},
	} {
		t.Run(tc.name+".All", func(t *testing.T) {
			_, err := tc.s.All(ctx, mockExec{dialectName: "mock-nocap"})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})

		t.Run(tc.name+".Count", func(t *testing.T) {
			_, err := tc.s.Count(ctx, mockExec{dialectName: "mock-nocap"})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestJoinCapabilityGate drives every RIGHT/FULL JOIN entry point through
// dialects that lack the capability -- a base-only dialect and the real
// sqlite dialect pinned to 3.38.0 (which has JoinCapabilities but reports
// both methods false below the 3.39.0 floor) -- asserting the typed
// ErrUnsupportedByDialect, never a panic or a silent wrong-SQL fallback.
// The same entry points must pass on postgres and on the default sqlite
// dialect (both capabilities true), and an INNER JOIN is never gated.
func TestJoinCapabilityGate(t *testing.T) {
	ctx := t.Context()

	left := From[joinUser](joinUsers)

	// Stream hands the gate error through the iterator, not a call return.
	streamErr := func(it iter.Seq2[Row2[Option[joinUser], joinOrder], error]) error {
		for _, err := range it {
			return err
		}

		return nil
	}

	tests := []struct {
		name string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"RightJoin2.All", func(ctx context.Context, e mockExec) error {
			_, err := RightJoinOn(left, userOrdersRel).All(ctx, e)

			return err
		}},
		{"RightJoin2.Stream", func(ctx context.Context, e mockExec) error {
			return streamErr(RightJoinOn(left, userOrdersRel).Stream(ctx, e))
		}},
		{"FullJoin2.All", func(ctx context.Context, e mockExec) error {
			_, err := FullJoinOn(left, userOrdersRel).All(ctx, e)

			return err
		}},
		{"FullJoin2.Stream", func(ctx context.Context, e mockExec) error {
			var err error

			for _, err = range FullJoinOn(left, userOrdersRel).Stream(ctx, e) {
				return err
			}

			return err
		}},
		{"JoinOn.RightJoin.All", func(ctx context.Context, e mockExec) error {
			_, err := JoinOn(left, userOrdersRel, RightJoin).All(ctx, e)

			return err
		}},
		{"JoinOn.FullJoin.All", func(ctx context.Context, e mockExec) error {
			_, err := JoinOn(left, userOrdersRel, FullJoin).All(ctx, e)

			return err
		}},
	}

	for _, tc := range tests {
		for _, d := range []string{"mock-nocap", "sqlite-3.38"} {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				err := tc.run(ctx, mockExec{dialectName: d})
				if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
					t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
				}
			})
		}
	}

	// Postgres and the default sqlite dialect (bundled 3.46.0) support
	// both: the gate passes, and (with mockExec's empty result set) All
	// returns no rows rather than an error.
	for _, tc := range []struct {
		name string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"RightJoin2.All", func(ctx context.Context, e mockExec) error {
			_, err := RightJoinOn(left, userOrdersRel).All(ctx, e)

			return err
		}},
		{"FullJoin2.All", func(ctx context.Context, e mockExec) error {
			_, err := FullJoinOn(left, userOrdersRel).All(ctx, e)

			return err
		}},
	} {
		for _, d := range []string{"postgres", "sqlite"} {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				if err := tc.run(ctx, mockExec{dialectName: d}); err != nil {
					t.Fatalf("err = %v, want nil (%s supports RIGHT/FULL JOIN)", err, d)
				}
			})
		}
	}

	// Inner join is never gated, even on a base-only dialect.
	if _, err := JoinOn(left, userOrdersRel, InnerJoin).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("InnerJoin on a base-only dialect failed: %v (INNER JOIN is universal SQL and must not be gated)", err)
	}
}

// TestCTEMaterializationTruthTable pins the MATERIALIZED / NOT MATERIALIZED
// capability matrix: Postgres 12+ and SQLite 3.35+ both support the modifier
// (the siblings' New() defaults report true). The gate turns an unsupported
// request into a typed
// dialect.ErrUnsupportedByDialect rather than rendering invalid SQL.
func TestCTEMaterializationTruthTable(t *testing.T) {
	if !postgres.New().SupportsCTEMaterialized() {
		t.Fatal("postgres.New() SupportsCTEMaterialized() = false, want true (defaults to 16.0)")
	}

	if !sqlite.New().SupportsCTEMaterialized() {
		t.Fatal("sqlite.New() SupportsCTEMaterialized() = false, want true (bundled 3.46.0)")
	}
}

// TestCTESearchCycleTruthTable pins the recursive SEARCH/CYCLE capability
// matrix: Postgres 14+ only; SQLite reports false at every version.
func TestCTESearchCycleTruthTable(t *testing.T) {
	if !postgres.New().SupportsCTESearchCycle() {
		t.Fatal("postgres.New() SupportsCTESearchCycle() = false, want true (defaults to 16.0)")
	}

	if sqlite.New().SupportsCTESearchCycle() {
		t.Fatal("sqlite.New() SupportsCTESearchCycle() = true, want false")
	}
}

// TestOrderedAggregateCapabilityTruthTable pins the ordered-argument
// aggregate capability matrix: array_agg/string_agg are Postgres-only,
// group_concat is SQLite-only, and the in-aggregate ORDER BY is
// version-gated to SQLite 3.44.0 while Postgres supports it at every
// version.
func TestOrderedAggregateCapabilityTruthTable(t *testing.T) {
	p := postgres.New()
	if !p.SupportsArrayAgg() || !p.SupportsStringAgg() || p.SupportsGroupConcat() || !p.SupportsOrderedAggregates() {
		t.Fatalf("postgres ordered-agg capabilities = (array %v, string %v, group %v, ordered %v), want (true, true, false, true)",
			p.SupportsArrayAgg(), p.SupportsStringAgg(), p.SupportsGroupConcat(), p.SupportsOrderedAggregates())
	}

	s := sqlite.New()
	if s.SupportsArrayAgg() || s.SupportsStringAgg() || !s.SupportsGroupConcat() || !s.SupportsOrderedAggregates() {
		t.Fatalf("sqlite.New() ordered-agg capabilities = (array %v, string %v, group %v, ordered %v), want (false, false, true, true)",
			s.SupportsArrayAgg(), s.SupportsStringAgg(), s.SupportsGroupConcat(), s.SupportsOrderedAggregates())
	}

	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"3.43.0", false},
		{"3.44.0", true},
		{"3.46.0", true},
	} {
		d, err := sqlite.NewWithVersion(tc.version)
		if err != nil {
			t.Fatalf("NewWithVersion(%q): %v", tc.version, err)
		}

		if got := d.SupportsOrderedAggregates(); got != tc.want {
			t.Errorf("sqlite %s SupportsOrderedAggregates() = %v, want %v", tc.version, got, tc.want)
		}

		if !d.SupportsGroupConcat() {
			t.Errorf("sqlite %s SupportsGroupConcat() = false, want true (universal)", tc.version)
		}
	}
}

// TestCTEExtensionCapabilityGate drives the MATERIALIZED modifier and the
// recursive SEARCH/CYCLE clauses through dialects that lack each capability,
// asserting the typed dialect.ErrUnsupportedByDialect (never a panic or
// silent-drop), and through the dialects that support them asserting the
// gate passes. A SEARCH/CYCLE clause on a plain (non-recursive) CTE is a
// typed caller error (ErrCTEClauseRequiresRecursive) independent of dialect.
func TestCTEExtensionCapabilityGate(t *testing.T) {
	ctx := t.Context()

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	orderCol, err := NewCTEName("ordercol")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	isCycle, err := NewCTEName("is_cycle")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	pathCol, err := NewCTEName("path")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	q := From(widgets)
	recursiveRel := NewRelation[widget, widget]("parent", "id", CTETable[widget](name))

	materialized := []struct {
		name string
		d    string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"With.Materialized", "mock-norec", func(ctx context.Context, e mockExec) error {
			_, err := With(name, q).Materialized().All(ctx, e)

			return err
		}},
		{"With.NotMaterialized", "mock-norec", func(ctx context.Context, e mockExec) error {
			_, err := With(name, q).NotMaterialized().All(ctx, e)

			return err
		}},
	}

	for _, tc := range materialized {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(ctx, mockExec{dialectName: tc.d}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}

	if _, err := With(name, q).Materialized().All(ctx, mockExec{dialectName: "sqlite"}); err != nil {
		t.Fatalf("Materialized on sqlite failed: %v (SQLite 3.35+ supports it)", err)
	}

	searchCycle := []struct {
		name string
		d    string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"SearchDepthFirst/sqlite", "sqlite", func(ctx context.Context, e mockExec) error {
			_, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).SearchDepthFirst(orderCol, widgetID.Col()).All(ctx, e)

			return err
		}},
		{"Cycle/sqlite", "sqlite", func(ctx context.Context, e mockExec) error {
			_, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).Cycle(isCycle, pathCol, widgetID.Col()).All(ctx, e)

			return err
		}},
	}

	for _, tc := range searchCycle {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(ctx, mockExec{dialectName: tc.d}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}

	if _, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).SearchDepthFirst(orderCol, widgetID.Col()).All(ctx, mockExec{dialectName: "postgres"}); err != nil {
		t.Fatalf("SearchDepthFirst on postgres failed: %v (Postgres 14+ supports it)", err)
	}

	if _, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).Cycle(isCycle, pathCol, widgetID.Col()).All(ctx, mockExec{dialectName: "postgres"}); err != nil {
		t.Fatalf("Cycle on postgres failed: %v (Postgres 14+ supports it)", err)
	}

	// The clause belongs only on WithRecursive, regardless of dialect.
	if _, err := With(name, q).SearchDepthFirst(orderCol, widgetID.Col()).All(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrCTEClauseRequiresRecursive) {
		t.Fatalf("plain With + SearchDepthFirst err = %v, want errors.Is(err, ErrCTEClauseRequiresRecursive)", err)
	}

	if _, err := With(name, q).Cycle(isCycle, pathCol, widgetID.Col()).All(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrCTEClauseRequiresRecursive) {
		t.Fatalf("plain With + Cycle err = %v, want errors.Is(err, ErrCTEClauseRequiresRecursive)", err)
	}

	// A SEARCH/CYCLE clause with no BY columns is a typed caller error: it
	// would render invalid SQL, so it never reaches the server.
	if _, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).SearchDepthFirst(orderCol).All(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrCTEClauseEmptyColumns) {
		t.Fatalf("SearchDepthFirst with no BY columns err = %v, want errors.Is(err, ErrCTEClauseEmptyColumns)", err)
	}

	if _, err := WithRecursive(name, q, Predicate[widget]{}, recursiveRel).Cycle(isCycle, pathCol).All(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrCTEClauseEmptyColumns) {
		t.Fatalf("Cycle with no BY columns err = %v, want errors.Is(err, ErrCTEClauseEmptyColumns)", err)
	}
}
