package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// orgUnit is a self-referential org-chart fixture for the recursive CTE
// round trip: each unit's ParentID points at another org_units row, or is
// NULL for the root.
type orgUnit struct {
	ID       string
	Name     string
	ParentID Option[string]
}

func (o *orgUnit) Scan(row Row) error {
	return row.Scan(&o.ID, &o.Name, &o.ParentID)
}

var (
	orgUnits  = NewTable[orgUnit]("org_units", []string{"id", "name", "parent_id"})
	orgID     = NewColumn[orgUnit, string]("org_units", "id")
	orgName   = NewColumn[orgUnit, string]("org_units", "name")
	orgParent = NewNullableColumn[orgUnit, string]("org_units", "parent_id")
)

func newOrgUnitsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE org_units (id text, name text, parent_id text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct{ id, name, parent string }{
		{"u1", "CEO", ""},
		{"u2", "VP Eng", "u1"},
		{"u3", "Eng Lead", "u2"},
		{"u4", "Eng 1", "u3"},
		{"u5", "Eng 2", "u3"},
	}

	for _, r := range rows {
		var p any

		if r.parent != "" {
			p = r.parent
		}

		if _, err := conn.Exec(ctx, `INSERT INTO org_units (id, name, parent_id) VALUES (?, ?, ?)`, r.id, r.name, p); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}

// widgetOrder is the join partner for widgets in the CTE-join round trip:
// one widget can have many orders.
type widgetOrder struct {
	ID       string
	WidgetID string
	Amount   int64
}

func (o *widgetOrder) Scan(row Row) error {
	return row.Scan(&o.ID, &o.WidgetID, &o.Amount)
}

var (
	widgetOrders = NewTable[widgetOrder]("widget_orders", []string{"id", "widget_id", "amount"})
	woAmount     = NewColumn[widgetOrder, int64]("widget_orders", "amount")
)

// seedWidgetOrders creates and fills the widget_orders table on an existing
// widgets-backed connection (newWidgetsDB): w1 has two orders, w2 one.
func seedWidgetOrders(ctx context.Context, t *testing.T, conn db.DB) {
	t.Helper()

	if _, err := conn.Exec(ctx, `CREATE TABLE widget_orders (id text, widget_id text, amount integer)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct {
		id, widget string
		amount     int64
	}{
		{"o1", "w1", 100},
		{"o2", "w1", 250},
		{"o3", "w2", 50},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO widget_orders (id, widget_id, amount) VALUES (?, ?, ?)`, r.id, r.widget, r.amount); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
}

func TestNewCTEName(t *testing.T) {
	valid := []string{"org_tree", "with_underscores", "Tree2"}
	for _, s := range valid {
		if _, err := NewCTEName(s); err != nil {
			t.Fatalf("NewCTEName(%q) = error %v, want nil", s, err)
		}
	}

	invalid := []string{"", "2fast", "bad-name", "has space", `quote"break`}
	for _, s := range invalid {
		if _, err := NewCTEName(s); err == nil {
			t.Fatalf("NewCTEName(%q) = nil error, want a validation error", s)
		}
	}
}

// TestWithRoundTrip proves a plain CTE against real SQLite: the inner
// query's WHERE renders inside the WITH body, and the outer query's
// WHERE/ORDER/LIMIT apply to the CTE result.
func TestWithRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, err := NewCTEName("big_widgets")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	got, err := With(name, From(widgets).Where(widgetQty.Gt(10))).
		Where(widgetQty.Lte(30)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 || got[0].ID != "w2" || got[1].ID != "w3" {
		t.Fatalf("All = %v, want w2,w3 (qty>10 from the body AND qty<=30 from the outer query)", got)
	}
}

func TestWithCountRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, _ := NewCTEName("big_widgets")

	n, err := With(name, From(widgets).Where(widgetQty.Gt(10))).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}
}

// TestWithFirstRoundTrip proves First applies an implicit LIMIT 1 to the
// outer query.
func TestWithFirstRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, _ := NewCTEName("all_widgets")

	row, ok, err := With(name, From(widgets)).OrderBy(widgetID.Asc()).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if !ok || row.ID != "w1" {
		t.Fatalf("First = (%v, %v), want w1", row, ok)
	}
}

// TestWithRecursiveRoundTrip is the recursive-CTE proof against real
// SQLite: WithRecursive over a self-referential org chart returns every
// unit reachable from the root, walking the whole tree through one
// `base UNION ALL recursive` CTE.
func TestWithRecursiveRoundTrip(t *testing.T) {
	ctx, conn := newOrgUnitsDB(t)

	name, err := NewCTEName("org_tree")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	// The recursive branch joins org_units back to the CTE being defined:
	// "org_units.parent_id = org_tree.id" -- the child side is a CTETable
	// reference to the very CTE this WithRecursive creates. The key names
	// come from the typed columns (Col().Name()), never bare strings.
	recursiveRel := NewRelation[orgUnit, orgUnit](orgParent.Col().Name(), orgID.Col().Name(), CTETable[orgUnit](name))

	got, err := WithRecursive(name, From(orgUnits).Where(orgParent.IsNull()), Predicate[orgUnit]{}, recursiveRel).
		OrderBy(orgName.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 5 {
		t.Fatalf("All returned %d units, want all 5 reachable from the root", len(got))
	}

	wantNames := []string{"CEO", "Eng 1", "Eng 2", "Eng Lead", "VP Eng"}
	for i, w := range wantNames {
		if got[i].Name != w {
			t.Fatalf("got[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}

	// Every non-root unit must carry its parent link through the recursion.
	for _, u := range got {
		if u.ID == "u1" {
			continue
		}

		if _, ok := u.ParentID.Get(); !ok {
			t.Fatalf("unit %s lost its parent link through the recursive CTE", u.ID)
		}
	}
}

// TestWithRecursiveCountRoundTrip proves Count over a recursive CTE.
func TestWithRecursiveCountRoundTrip(t *testing.T) {
	ctx, conn := newOrgUnitsDB(t)

	name, _ := NewCTEName("org_tree")
	recursiveRel := NewRelation[orgUnit, orgUnit](orgParent.Col().Name(), orgID.Col().Name(), CTETable[orgUnit](name))

	n, err := WithRecursive(name, From(orgUnits).Where(orgParent.IsNull()), Predicate[orgUnit]{}, recursiveRel).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 5 {
		t.Fatalf("Count = %d, want 5", n)
	}
}

// TestWithJoinRoundTrip wraps a Join2 in a CTE and reads the joined rows
// back through the CTE's name: widgets with their widget_orders.
func TestWithJoinRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	name, _ := NewCTEName("w_orders")

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	got, err := WithJoin(name, JoinOn(From(widgets), rel, InnerJoin)).
		WhereRight(woAmount.Gt(75)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// Orders with amount > 75: o1 (w1, 100), o2 (w1, 250) -- o3 (w2, 50)
	// excluded by the outer WHERE.
	if len(got) != 2 {
		t.Fatalf("All returned %d rows, want 2", len(got))
	}

	if got[0].A.ID != "w1" || got[0].B.Amount != 100 {
		t.Fatalf("got[0] = %+v, want {w1 100}", got[0])
	}

	if got[1].A.ID != "w1" || got[1].B.Amount != 250 {
		t.Fatalf("got[1] = %+v, want {w1 250}", got[1])
	}
}

// TestWithGroupedRoundTrip wraps a GroupBy/Agg query in a CTE and reads the
// grouped rows back through the CTE's name.
func TestWithGroupedRoundTrip(t *testing.T) {
	ctx, conn := newCategorizedWidgetsDB(t)

	name, _ := NewCTEName("cats")

	type row struct {
		category string
		count    int64
		total    int64
	}

	var got []row

	err := WithGrouped(name, From(widgets).GroupBy(widgetCategory.Col()).Agg(Count(), Sum[int64](widgetQty))).
		OrderBy(widgetCategory.Asc()).
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

	if len(got) != 2 {
		t.Fatalf("Scan returned %d groups, want 2 (fruit, tool)", len(got))
	}

	if got[0].category != "fruit" || got[0].count != 3 || got[0].total != 60 {
		t.Fatalf("got[0] = %+v, want {fruit 3 60}", got[0])
	}

	if got[1].category != "tool" || got[1].count != 1 || got[1].total != 5 {
		t.Fatalf("got[1] = %+v, want {tool 1 5}", got[1])
	}
}

// TestWithMaterializedRoundTrip proves the MATERIALIZED hint against real
// SQLite 3.46 (which supports it): the result set is unchanged, the clause
// is valid SQL, and the copy-on-write chain still composes with Where/OrderBy.
func TestWithMaterializedRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, err := NewCTEName("big_widgets")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	got, err := With(name, From(widgets).Where(widgetQty.Gt(10))).
		Materialized().
		Where(widgetQty.Lte(30)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 || got[0].ID != "w2" || got[1].ID != "w3" {
		t.Fatalf("All = %v, want w2,w3", got)
	}
}

// TestWithNotMaterializedRoundTrip proves the NOT MATERIALIZED hint against
// real SQLite; like the MATERIALIZED form the result set is unchanged.
func TestWithNotMaterializedRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, _ := NewCTEName("big_widgets")

	n, err := With(name, From(widgets).Where(widgetQty.Gt(10))).NotMaterialized().Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}
}

// TestWithRecursiveSearchCycleUnsupportedOnSQLite proves SQLite (which does
// support WITH RECURSIVE and CTE MATERIALIZED) still rejects the Postgres-only
// SEARCH/CYCLE clauses with the typed dialect.ErrUnsupportedByDialect.
func TestWithRecursiveSearchCycleUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newOrgUnitsDB(t)

	name, err := NewCTEName("org_tree")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	orderCol, _ := NewCTEName("ordercol")
	isCycle, _ := NewCTEName("is_cycle")
	pathCol, _ := NewCTEName("path")

	recursiveRel := NewRelation[orgUnit, orgUnit](orgParent.Col().Name(), orgID.Col().Name(), CTETable[orgUnit](name))

	_, err = WithRecursive(name, From(orgUnits).Where(orgParent.IsNull()), Predicate[orgUnit]{}, recursiveRel).
		SearchDepthFirst(orderCol, orgID.Col()).
		All(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("SearchDepthFirst err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	_, err = WithRecursive(name, From(orgUnits).Where(orgParent.IsNull()), Predicate[orgUnit]{}, recursiveRel).
		Cycle(isCycle, pathCol, orgID.Col()).
		All(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Cycle err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestWithJoinMaterializedRoundTrip proves the MATERIALIZED hint on a join
// CTE against real SQLite and that its rows scan unchanged.
func TestWithJoinMaterializedRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	name, _ := NewCTEName("w_orders")

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	got, err := WithJoin(name, JoinOn(From(widgets), rel, InnerJoin)).
		Materialized().
		WhereRight(woAmount.Gt(75)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 || got[0].A.ID != "w1" || got[0].B.Amount != 100 {
		t.Fatalf("got = %+v, want two w1 rows ordered by amount", got)
	}
}

// TestCTEQueryWhereDoesNotMutateBase proves CTEQuery follows the same
// copy-on-write discipline as Query[T, PT].
func TestCTEQueryWhereDoesNotMutateBase(t *testing.T) {
	name, _ := NewCTEName("w")
	base := With(name, From(widgets))

	branch := base.Where(widgetQty.Gt(10))

	if base.where.IsSet() {
		t.Fatalf("base.where.IsSet() = true after branching, want false (unmodified)")
	}

	if !branch.where.IsSet() {
		t.Fatalf("branch.where.IsSet() = false, want true")
	}
}

// TestCTEQueryOrderByDoesNotShareBackingArray proves OrderBy copies into a
// fresh backing array, matching Query[T, PT].OrderBy's branch-safety rule.
func TestCTEQueryOrderByDoesNotShareBackingArray(t *testing.T) {
	name, _ := NewCTEName("w")
	base := With(name, From(widgets)).OrderBy(widgetID.Asc())

	branchA := base.OrderBy(widgetName.Asc())
	branchB := base.OrderBy(widgetQty.Desc())

	if len(base.order) != 1 {
		t.Fatalf("base.order mutated by branching: %v", base.order)
	}

	if len(branchA.order) != 2 || branchA.order[1].Column.Name() != "name" {
		t.Fatalf("branchA.order = %v, want [id, name]", branchA.order)
	}

	if len(branchB.order) != 2 || branchB.order[1].Column.Name() != "quantity" {
		t.Fatalf("branchB.order = %v, want [id, quantity]", branchB.order)
	}

	if branchA.order[1].Column.Name() == branchB.order[1].Column.Name() {
		t.Fatalf("branchA and branchB unexpectedly share an order entry")
	}
}

// TestCTEJoinChainModifiers proves CTEJoin's outer-query chain composes:
// doubly-set filters, both ORDER BY lists, Limit and both materialization
// hints render, and the no-filter form runs end to end.
func TestCTEJoinChainModifiers(t *testing.T) {
	ctx, conn := newJoinDB(t)

	name, err := NewCTEName("j")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)

	plain, err := WithJoin(name, inner).All(ctx, conn)
	if err != nil {
		t.Fatalf("plain WithJoin.All: %v", err)
	}

	if len(plain) != 3 {
		t.Fatalf("plain WithJoin rows = %d, want 3", len(plain))
	}

	chained := WithJoin(name, inner).
		Where(joinUserMail.Eq("a@example.com")).Where(joinUserMail.Neq("nobody@example.com")).
		WhereRight(joinOrderCents.Gt(int64(50))).WhereRight(joinOrderCents.Lt(int64(1000))).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		Limit(1).
		Materialized()

	rows, err := chained.All(ctx, conn)
	if err != nil {
		t.Fatalf("chained WithJoin.All: %v", err)
	}

	if len(rows) != 1 || rows[0].A.ID != "u1" {
		t.Fatalf("chained WithJoin rows = %+v, want 1 row for u1", rows)
	}

	nm := WithJoin(name, inner).NotMaterialized()

	if _, err := nm.All(ctx, conn); err != nil {
		t.Fatalf("NotMaterialized WithJoin.All: %v", err)
	}
}

// TestCTEJoinGateErrors proves CTEJoin renderOuter fails closed: no CTE
// capability, no materialization support, and a gated join keyword in the
// body each surface typed errors before any SQL is issued.
func TestCTEJoinGateErrors(t *testing.T) {
	ctx := t.Context()

	name, err := NewCTEName("j")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)

	if _, err := WithJoin(name, inner).All(ctx, mockExec{dialectName: "mock-nocap"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("WithJoin on mock-nocap err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := WithJoin(name, inner).Materialized().All(ctx, mockExec{dialectName: "mock-nocap"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Materialized WithJoin on mock-nocap err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	rightInner := JoinOn(From[joinUser](joinUsers), userOrdersRel, RightJoin)

	if _, err := WithJoin(name, rightInner).All(ctx, mockExec{dialectName: "sqlite-3.38"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("RIGHT-body WithJoin on sqlite-3.38 err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := WithJoin(name, inner).All(ctx, fakeDB{}); err == nil {
		t.Fatal("WithJoin.All on an unresolvable dialect succeeded, want an error")
	}
}

// TestCTEQueryOffsetFirstCountEdges proves CTEQuery's Offset chain, First's
// empty-result contract, and Count/All error paths.
func TestCTEQueryOffsetFirstCountEdges(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	q := From(widgets).Where(widgetQty.Gt(int64(5)))

	off, err := With(name, q).Limit(10).Offset(1).All(ctx, conn)
	if err != nil {
		t.Fatalf("Offset All: %v", err)
	}

	if len(off) != 2 {
		t.Fatalf("Offset All rows = %d, want 2", len(off))
	}

	row, ok, err := With(name, From(widgets).Where(widgetQty.Gt(int64(1000)))).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if ok || row != nil {
		t.Fatalf("First = (%+v, %v), want (nil, false) (no rows)", row, ok)
	}

	if _, _, err := With(name, q).First(ctx, fakeDB{}); err == nil {
		t.Fatal("First on an unresolvable dialect succeeded, want an error")
	}

	if _, err := With(name, q).Count(ctx, fakeDB{}); err == nil {
		t.Fatal("Count on an unresolvable dialect succeeded, want an error")
	}

	if _, err := With(name, q).All(ctx, fakeDB{}); err == nil {
		t.Fatal("All on an unresolvable dialect succeeded, want an error")
	}

	boom := errors.New("boom")
	stub := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	if _, err := With(name, q).All(ctx, stub); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := With(name, q).Count(ctx, stub); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}
}

// TestCTEGroupedMaterializationAndAliasGate proves the grouped-CTE
// materialization hints render and the empty-alias guard fails closed.
func TestCTEGroupedMaterializationAndAliasGate(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, err := NewCTEName("g")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := From(widgets).GroupBy(widgetName.Col()).Agg(Count())

	for _, tc := range []struct {
		name string
		q    CTEGroupedQuery[widget]
	}{
		{"materialized", WithGrouped(name, inner).Materialized()},
		{"not-materialized", WithGrouped(name, inner).NotMaterialized()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := 0

			err := tc.q.Scan(ctx, conn, func(Row) error {
				groups++

				return nil
			})
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}

			if groups != 3 {
				t.Fatalf("groups = %d, want 3", groups)
			}
		})
	}

	aliasless := From(widgets).GroupBy(widgetName.Col()).Agg(Aggregate{Func: AggCount})

	if err := WithGrouped(name, aliasless).Scan(ctx, conn, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan with an alias-less aggregate succeeded, want an error")
	}
}

// TestCTEQueryExecutionErrorPaths drives render, scan, iteration and
// close failures through CTEQuery All/Count.
func TestCTEQueryExecutionErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	q := From(widgets).Where(widgetQty.Gt(int64(5)))
	multi := From(widgetOrders)

	if _, err := With(name, From(widgets).Where(widgetID.InSub(multi))).All(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("All with a multi-column IN subquery succeeded, want a render error")
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: rows}
	}

	scanErr := newStub(&stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first"}}, scanErr: boom})

	if _, err := With(name, q).All(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first"}}, iterErr: boom})

	if _, err := With(name, q).All(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if _, err := With(name, q).All(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := With(name, q).Materialized().Count(ctx, mockExec{dialectName: "mock-norec"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Materialized Count on mock-norec err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := With(name, From(widgets).Where(widgetID.InSub(multi))).Count(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("Count with a multi-column IN subquery succeeded, want a render error")
	}

	countScan := newStub(&stubRows{values: [][]any{{int64(3)}}, scanErr: boom})

	if _, err := With(name, q).Count(ctx, countScan); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}

	countIter := newStub(&stubRows{values: [][]any{{int64(3)}}, iterErr: boom})

	if _, err := With(name, q).Count(ctx, countIter); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}

	countClose := newStub(&stubRows{closeErr: boom})

	if _, err := With(name, q).Count(ctx, countClose); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}
}

// TestCTEJoinExecutionErrorPaths drives query, scan, iteration and close
// failures through CTEJoin.All, plus the extensions gate and the
// left-only outer filter.
func TestCTEJoinExecutionErrorPaths(t *testing.T) {
	ctx, conn := newJoinDB(t)
	boom := errors.New("boom")

	name, err := NewCTEName("j")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)

	filtered, err := WithJoin(name, inner).Where(joinUserMail.Eq("a@example.com")).All(ctx, conn)
	if err != nil {
		t.Fatalf("left-only WithJoin.All: %v", err)
	}

	if len(filtered) != 3 {
		t.Fatalf("left-only WithJoin rows = %d, want 3", len(filtered))
	}

	if _, err := WithJoin(name, inner).Materialized().All(ctx, mockExec{dialectName: "mock-norec"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Materialized WithJoin on mock-norec err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: rows}
	}

	queryErr := newStub(&stubRows{})
	queryErr.queryErr = boom

	if _, err := WithJoin(name, inner).All(ctx, queryErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	scanErr := newStub(&stubRows{values: [][]any{{"u1", "a@example.com", "o1", "u1", int64(100)}}, scanErr: boom})

	if _, err := WithJoin(name, inner).All(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{values: [][]any{{"u1", "a@example.com", "o1", "u1", int64(100)}}, iterErr: boom})

	if _, err := WithJoin(name, inner).All(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if _, err := WithJoin(name, inner).All(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}
}

// TestCTEGroupedScanErrorPaths drives resolve, extensions, query,
// row-callback, iteration and close failures through CTEGroupedQuery.Scan.
func TestCTEGroupedScanErrorPaths(t *testing.T) {
	ctx, _ := newWidgetsDB(t)
	boom := errors.New("boom")

	name, err := NewCTEName("g")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := From(widgets).GroupBy(widgetName.Col()).Agg(Count())
	g := WithGrouped(name, inner)
	noop := func(Row) error { return nil }

	if err := g.Scan(ctx, fakeDB{}, noop); err == nil {
		t.Fatal("Scan on an unresolvable dialect succeeded, want an error")
	}

	if err := WithGrouped(name, inner).Materialized().Scan(ctx, mockExec{dialectName: "mock-norec"}, noop); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Materialized Scan on mock-norec err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: rows}
	}

	queryErr := newStub(&stubRows{})
	queryErr.queryErr = boom

	if err := g.Scan(ctx, queryErr, noop); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	fnErr := newStub(&stubRows{values: [][]any{{"Alpha", int64(1)}}})

	if err := g.Scan(ctx, fnErr, func(Row) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{values: [][]any{{"Alpha", int64(1)}}, iterErr: boom})

	if err := g.Scan(ctx, iterErr, noop); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if err := g.Scan(ctx, closeErr, noop); !errors.Is(err, boom) {
		t.Fatalf("Scan err = %v, want errors.Is(err, boom)", err)
	}
}

// TestCTEQueryWhereDoubleCombines proves CTEQuery.Where ANDs a second
// predicate into the first.
func TestCTEQueryWhereDoubleCombines(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	name, err := NewCTEName("w")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	rows, err := With(name, From(widgets)).Where(widgetQty.Gt(int64(5))).Where(widgetName.Neq("Gamma")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (w1, w2)", len(rows))
	}
}

// TestCTEGroupedBodyRenderError proves a grouped-CTE body that cannot
// render (an ordered aggregate rejected at construction) fails at render
// time rather than emitting invalid SQL.
func TestCTEGroupedBodyRenderError(t *testing.T) {
	ctx := t.Context()

	name, err := NewCTEName("g")
	if err != nil {
		t.Fatalf("NewCTEName: %v", err)
	}

	inner := From(widgets).GroupBy(widgetName.Col()).Agg(GroupConcat(widgetName, ",", ";"))

	if err := WithGrouped(name, inner).Scan(ctx, mockExec{dialectName: "sqlite"}, func(Row) error { return nil }); err == nil {
		t.Fatal("Scan with a preset-bad ordered aggregate succeeded, want a render error")
	}
}
