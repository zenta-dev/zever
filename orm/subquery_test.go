package orm

import (
	"context"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// The widget_orders fixture entity and table (widgetOrder, widgetOrders,
// woAmount) are declared by cte_test.go and shared here; subquery tests add
// the two column references woAmount's family lacks.
var (
	orderID       = NewColumn[widgetOrder, string]("widget_orders", "id")
	orderWidgetID = NewColumn[widgetOrder, string]("widget_orders", "widget_id")
)

// newWidgetOrdersDB opens an in-memory sqlite database seeded with the same
// three widgets as newWidgetsDB plus a widget_orders child table:
//
//	o1 -> w1 amount 10 | o2 -> w1 amount 15
//	o3 -> w2 amount 25 | o4 -> w3 amount 35
//
// widget quantities (10/20/30) deliberately mirror order amounts where a
// scalar-subquery comparison needs them to line up.
func newWidgetOrdersDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text)`); err != nil {
		t.Fatalf("create widgets: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE widget_orders (id text, widget_id text, amount integer)`); err != nil {
		t.Fatalf("create widget_orders: %v", err)
	}

	rows := []struct {
		id, name string
		qty      int64
		bio      any
	}{
		{"w1", "Alpha", 10, "first"},
		{"w2", "Beta", 20, nil},
		{"w3", "Gamma", 30, "third"},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, r.id, r.name, r.qty, r.bio); err != nil {
			t.Fatalf("insert widget: %v", err)
		}
	}

	orders := []struct {
		id, widgetID string
		amount       int64
	}{
		{"o1", "w1", 10},
		{"o2", "w1", 15},
		{"o3", "w2", 25},
		{"o4", "w3", 35},
	}

	for _, o := range orders {
		if _, err := conn.Exec(ctx, `INSERT INTO widget_orders (id, widget_id, amount) VALUES (?, ?, ?)`, o.id, o.widgetID, o.amount); err != nil {
			t.Fatalf("insert order: %v", err)
		}
	}

	return ctx, conn
}

func wantIDs(t *testing.T, got []*widget, want ...string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d rows %v, want %v", len(got), widgetIDs(got), want)
	}

	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("row %d = %s, want %s (got %v)", i, got[i].ID, id, widgetIDs(got))
		}
	}
}

// TestColumnInSub is the codegen-style composite shape: widgets whose id is
// IN the ids an inner query over a DIFFERENT entity (widget_orders) selects
// -- the inner query projects exactly one column via Query.Columns.
func TestColumnInSub(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).
		Where(woAmount.Gt(20)).
		Columns(orderWidgetID.Col())

	got, err := From(widgets).
		Where(widgetID.InSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w2", "w3")
}

func TestColumnNotInSub(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).
		Where(woAmount.Gt(20)).
		Columns(orderWidgetID.Col())

	got, err := From(widgets).
		Where(widgetID.NotInSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

// TestColumnNotInSubEmptyInner proves the NOT IN (empty-from-query) shape:
// a widget with no orders falls through both IN and NOT IN filters of an
// empty inner result the same way SQL defines NOT IN.
func TestColumnNotInSubEmptyInner(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	none := From(widgetOrders).Where(orderID.Eq("nope")).Columns(orderWidgetID.Col())

	got, err := From(widgets).
		Where(widgetID.NotInSub(none)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// NOT IN on an empty set is vacuously TRUE in SQL, so every widget
	// remains.
	wantIDs(t, got, "w1", "w2", "w3")
}

// TestExists documents orm's non-correlated EXISTS semantics: the inner
// query is evaluated independently (its Where carries no Outer/OuterNullable
// reference, unlike the correlated EXISTS tests in correlated_test.go), so
// its truth value is the same for every outer row. A satisfied inner query
// thus flags all outer rows, not just the matching widget.
func TestExists(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	matched := From(widgetOrders).Where(orderWidgetID.Eq("w2"))
	satisfied, err := From(widgets).
		Where(Exists[widget](matched)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, satisfied, "w1", "w2", "w3")

	none := From(widgetOrders).Where(orderWidgetID.Eq("missing"))
	unsatisfied, err := From(widgets).
		Where(Exists[widget](none)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All unsatisfied: %v", err)
	}

	if len(unsatisfied) != 0 {
		t.Fatalf("unsatisfied EXISTS returned rows %v, want none", widgetIDs(unsatisfied))
	}
}

func TestNotExists(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	// No order amounts above 100 exist, so the existential is FALSE for
	// every outer row; NOT flips the whole filter to TRUE.
	inner := From(widgetOrders).Where(woAmount.Gt(100))

	got, err := From(widgets).
		Where(NotExists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1", "w2", "w3")

	// When the existential IS satisfied, NOT EXISTS returns no rows.
	satisfied := From(widgetOrders).Where(orderID.Eq("o1"))
	blocked, err := From(widgets).
		Where(NotExists[widget](satisfied)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All blocked: %v", err)
	}

	if len(blocked) != 0 {
		t.Fatalf("satisfied NOT EXISTS returned rows %v, want none", widgetIDs(blocked))
	}
}

// TestSubqueryComposesInCompoundChain proves subquery predicates nest inside
// And/Or/Not and combine with plain bound-value predicates on the SAME outer
// query -- the outer args and inner args flow through one statement.
func TestSubqueryComposesInCompoundChain(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	bigOrders := From(widgetOrders).Where(woAmount.Gt(20)).Columns(orderWidgetID.Col())

	// quantity >= 20 AND (id IN (SELECT widget_id ... amount > 20) OR name = 'Gamma')
	got, err := From(widgets).
		Where(And(
			widgetQty.Gte(20),
			Or(widgetID.InSub(bigOrders), widgetName.Eq("Gamma")),
		)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// w2 (qty 20, in big orders), w3 (qty 30, in big orders AND named Gamma).
	wantIDs(t, got, "w2", "w3")

	negated, err := From(widgets).
		Where(Not(widgetID.InSub(bigOrders))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All negated: %v", err)
	}

	wantIDs(t, negated, "w1")
}

// TestScalarSubqueryComparisons round-trips the EqScalar..LteScalar family
// against a one-row inner query projecting its single column.
func TestScalarSubqueryComparisons(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	amountOf := func(orderIDValue string) Query[widgetOrder, *widgetOrder] {
		return From(widgetOrders).Where(orderID.Eq(orderIDValue)).Columns(woAmount.Col())
	}

	tests := []struct {
		name string
		pred Predicate[widget]
		want []string
	}{
		{"eq", widgetQty.EqScalar(amountOf("o1")), []string{"w1"}}, // qty == amount(o1)=10
		{"neq", widgetQty.NeqScalar(amountOf("o1")), []string{"w2", "w3"}},
		{"gt", widgetQty.GtScalar(amountOf("o3")), []string{"w3"}},         // qty > 25
		{"gte", widgetQty.GteScalar(amountOf("o2")), []string{"w2", "w3"}}, // qty >= 15
		{"lt", widgetQty.LtScalar(amountOf("o3")), []string{"w1", "w2"}},   // qty < 25
		{"lte", widgetQty.LteScalar(amountOf("o1")), []string{"w1"}},       // qty <= 10
	}

	for _, tc := range tests {
		got, err := From(widgets).Where(tc.pred).OrderBy(widgetID.Asc()).All(ctx, conn)
		if err != nil {
			t.Fatalf("%s: All: %v", tc.name, err)
		}

		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %d rows %v, want %v", tc.name, len(got), widgetIDs(got), tc.want)
		}

		wantIDs(t, got, tc.want...)
	}
}

// TestNullableColumnInSub covers the NullableColumn subquery surface through
// the NULL-valued bio column. The inner query projects w3's bio ('third'),
// so the NULL-valued w2 row is excluded -- SQL evaluates `NULL IN (...)` as
// NULL, never TRUE -- and only w3 (whose bio matches) survives.
func TestNullableColumnInSub(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgets).Where(widgetID.Eq("w3")).Columns(widgetBio.Col())

	got, err := From(widgets).
		Where(widgetBio.InSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w3")
}

// TestInSubProjectionRequiredProves a multi-column inner query (no
// Query.Columns projection) is rejected with a typed rendering error at
// execution time rather than shipping invalid multi-column IN SQL.
func TestInSubProjectionRequired(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	unprojected := From(widgetOrders).Where(woAmount.Gt(20))

	_, err := From(widgets).Where(widgetID.InSub(unprojected)).All(ctx, conn)
	if err == nil {
		t.Fatalf("All with a multi-column InSub succeeded, want an error")
	}

	if err.Error() != "orm: Query.Stream: orm/render: IN-subquery must project exactly one column, got 3 (project one via the query's Columns selector)" {
		t.Fatalf("error = %v, want the single-column projection error", err)
	}
}

// TestQueryColumnsProjection pins Query.Columns' plumbing: the projected
// column list drives the inner SELECT (its rendering is proven at the
// render level and in every subquery round-trip above). In a subquery
// operand the inner rows are never scanned, so any projected set is
// valid; against a standalone All/Stream the codegen Scan reads the full
// entity positionally, so a subset changes the expected row shape.
func TestQueryColumnsProjection(t *testing.T) {
	q := From(widgetOrders).Where(woAmount.Gt(20)).Columns(orderWidgetID.Col())

	if got := q.selectColumns(); !reflect.DeepEqual(got, []string{"widget_id"}) {
		t.Fatalf("selectColumns() = %v, want [widget_id]", got)
	}
}
