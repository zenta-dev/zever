package orm

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// order_tag is a second child entity of widget_orders, used by the nested
// correlation (level-two) tests: an order_tags row references a widget_order
// by order_id and carries a nullable note column.
type orderTag struct {
	ID      string
	OrderID string
	Tag     string
	Note    Option[string]
}

func (t *orderTag) Scan(row Row) error {
	return row.Scan(&t.ID, &t.OrderID, &t.Tag, &t.Note)
}

var (
	orderTags       = NewTable[orderTag]("order_tags", []string{"id", "order_id", "tag", "note"})
	orderTagOrderID = NewColumn[orderTag, string]("order_tags", "order_id")
	orderTagTag     = NewColumn[orderTag, string]("order_tags", "tag")
	orderTagNote    = NewNullableColumn[orderTag, string]("order_tags", "note")
)

// newCorrelatedDB opens an in-memory sqlite database seeded with the same
// widgets and widget_orders as newWidgetOrdersDB (see subquery_test.go) plus
// an order_tags child table:
//
//	t1 -> o1 tag rush note Alpha | t2 -> o2 tag normal note NULL
//	t3 -> o3 tag rush note Beta | t4 -> o4 tag slow note NULL
//
// Widget names (Alpha/Beta/Gamma) deliberately mirror the tag notes where
// the nullable-correlation test needs them to line up.
func newCorrelatedDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text)`); err != nil {
		t.Fatalf("create widgets: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE widget_orders (id text, widget_id text, amount integer)`); err != nil {
		t.Fatalf("create widget_orders: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE order_tags (id text, order_id text, tag text, note text)`); err != nil {
		t.Fatalf("create order_tags: %v", err)
	}

	widgets := []struct {
		id, name string
		qty      int64
		bio      any
	}{
		{"w1", "Alpha", 10, "first"},
		{"w2", "Beta", 20, nil},
		{"w3", "Gamma", 30, "third"},
		// w4 has NO orders -- the orderless widget the NOT EXISTS anti-join
		// needs to actually have a positive branch to select.
		{"w4", "Delta", 15, nil},
	}

	for _, w := range widgets {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, w.id, w.name, w.qty, w.bio); err != nil {
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

	tags := []struct {
		id, orderID, tag string
		note             any
	}{
		{"t1", "o1", "rush", "Alpha"},
		{"t2", "o2", "normal", nil},
		{"t3", "o3", "rush", "Beta"},
		{"t4", "o4", "slow", nil},
	}

	for _, tag := range tags {
		if _, err := conn.Exec(ctx, `INSERT INTO order_tags (id, order_id, tag, note) VALUES (?, ?, ?, ?)`, tag.id, tag.orderID, tag.tag, tag.note); err != nil {
			t.Fatalf("insert order tag: %v", err)
		}
	}

	return ctx, conn
}

// TestCorrelatedNotExistsAntiJoin is the classic anti-join: every widget
// with ZERO orders -- the idiom that motivated this whole stream and was
// impossible before correlated references existed. Only w4 is orderless.
func TestCorrelatedNotExistsAntiJoin(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID)))

	got, err := From(widgets).
		Where(NotExists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w4")
}

// TestCorrelatedExistsPositive proves the positive side: every widget with
// AT LEAST one order, via the same correlated inner query.
func TestCorrelatedExistsPositive(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID)))

	got, err := From(widgets).
		Where(Exists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1", "w2", "w3")
}

// TestCorrelatedInSub is the correlated IN-subquery: a widget's quantity is
// IN the amounts its own orders charge -- the inner SELECT's WHERE correlates
// to the outer widget per row.
func TestCorrelatedInSub(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).
		Where(orderWidgetID.EqOuter(Outer(widgetID))).
		Columns(woAmount.Col())

	got, err := From(widgets).
		Where(widgetQty.InSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

// TestCorrelatedScalarComparison is the correlated scalar comparison: a
// widget's quantity equals its own LOWEST order amount. The inner scalar
// SELECT is correlated (amounts of THIS widget's orders), returns exactly
// one cell per widget (ORDER BY/LIMIT 1), and w1's order amounts {10, 15}
// make its lowest amount equal its quantity -- while w2 (lowest 25 vs
// quantity 20) and w3 (lowest 35 vs 30) differ, and w4 (no orders) yields
// an empty scalar, which SQL evaluates as NULL (never equal AND never
// unequal -- SQL's three-valued logic).
func TestCorrelatedScalarComparison(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	lowestOwnAmount := From(widgetOrders).
		Where(orderWidgetID.EqOuter(Outer(widgetID))).
		OrderBy(woAmount.Asc()).
		Limit(1).
		Columns(woAmount.Col())

	eq, err := From(widgets).
		Where(widgetQty.EqScalar(lowestOwnAmount)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All eq: %v", err)
	}

	wantIDs(t, eq, "w1")

	neq, err := From(widgets).
		Where(widgetQty.NeqScalar(lowestOwnAmount)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All neq: %v", err)
	}

	wantIDs(t, neq, "w2", "w3")
}

// TestCorrelatedWithOuterBoundArg proves a correlated predicate and a plain
// outer bound-arg predicate flow through ONE statement with the outer
// placeholder numbered before the inner one -- the SQLite round trip of the
// $N-ordering render test, and w2/w3's survival depends on the correlation
// actually binding (o2 belongs to w1, not to w2/w3).
func TestCorrelatedWithOuterBoundArg(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).Where(And(orderWidgetID.EqOuter(Outer(widgetID)), orderID.Eq("o2")))

	got, err := From(widgets).
		Where(And(widgetQty.Gte(20), NotExists[widget](inner))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w2", "w3")
}

// TestCorrelatedNotExistsComposesInCompoundChain proves And/Or/Not
// composition: a correlated NOT EXISTS inside an OR next to a plain value
// predicate, with a plain bound-arg predicate ANDed on the outside.
func TestCorrelatedNotExistsComposesInCompoundChain(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(widgetID)))

	got, err := From(widgets).
		Where(And(
			widgetQty.Gte(15),
			Or(NotExists[widget](inner), widgetName.Eq("Gamma")),
		)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// w1 (qty 10) fails qty >= 15. w2 (qty 20, has orders) fails the OR
	// (NOT EXISTS false, name Beta): dropped. w3 (qty 30, has orders but
	// named Gamma) passes the OR. w4 (qty 15, NO orders) passes NOT EXISTS.
	wantIDs(t, got, "w3", "w4")
}

// TestCorrelatedNested is the one-level-deep nested case: a correlated
// EXISTS whose subquery's IN-subquery correlates to the MIDDLE subquery's
// table (order_tags.order_id = widget_orders.id). The query returns the
// widgets with an order that carries a "rush" tag.
func TestCorrelatedNested(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	rushOrderIDs := From(orderTags).
		Where(And(orderTagTag.Eq("rush"), orderTagOrderID.EqOuter(Outer(orderID)))).
		Columns(orderTagOrderID.Col())

	inner := From(widgetOrders).
		Where(And(orderWidgetID.EqOuter(Outer(widgetID)), orderID.InSub(rushOrderIDs)))

	got, err := From(widgets).
		Where(Exists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1", "w2")
}

// TestCorrelatedNullableColumn covers the NullableColumn surface: an
// order_tags note (a NULLable inner column) correlated against the outer
// widget's name. w2's note comes back NULL -- SQL evaluates `NULL = name`
// as NULL, never true -- so w2 is excluded and only w1 and matching rows
// survive.
func TestCorrelatedNullableColumn(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(orderTags).Where(orderTagNote.EqOuter(Outer(widgetName)))

	got, err := From(widgets).
		Where(NotExists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// w1 (Alpha) and w2 (Beta) have a matching note; w3 (Gamma) and w4
	// (Delta) have none.
	wantIDs(t, got, "w3", "w4")
}

// TestOuterRefNodeShape pins the erased marker's shape: a Column.EqOuter
// predicate is an NBinary node whose LHS is the inner column and whose Value
// is the type-safe OuterRef carrying the outer column's table/name.
func TestOuterRefNodeShape(t *testing.T) {
	ops := []struct {
		name string
		op   Op
	}{
		{"EqOuter", Eq},
		{"NeqOuter", Neq},
		{"GtOuter", Gt},
		{"GteOuter", Gte},
		{"LtOuter", Lt},
		{"LteOuter", Lte},
	}

	for _, o := range ops {
		var p Predicate[widgetOrder]

		switch o.name { //nolint:dupl // one call per op, each one line
		case "EqOuter":
			p = orderWidgetID.EqOuter(Outer(widgetID))
		case "NeqOuter":
			p = orderWidgetID.NeqOuter(Outer(widgetID))
		case "GtOuter":
			p = orderWidgetID.GtOuter(Outer(widgetID))
		case "GteOuter":
			p = orderWidgetID.GteOuter(Outer(widgetID))
		case "LtOuter":
			p = orderWidgetID.LtOuter(Outer(widgetID))
		case "LteOuter":
			p = orderWidgetID.LteOuter(Outer(widgetID))
		}

		n := p.Render()

		if n.Kind != NBinary || n.Table != "widget_orders" || n.Column != "widget_id" {
			t.Fatalf("%s: node LHS = kind %d table %q column %q, want NBinary widget_orders/widget_id", o.name, n.Kind, n.Table, n.Column)
		}

		if n.Op != o.op {
			t.Fatalf("%s: Op = %d, want %d", o.name, n.Op, o.op)
		}

		ref, ok := n.Value.(OuterRef[widget, string])
		if !ok {
			t.Fatalf("%s: Value type %T, want OuterRef[widget,string]", o.name, n.Value)
		}

		if ref.table != "widgets" || ref.name != "id" {
			t.Fatalf("%s: outer ref carries table %q column %q, want widgets/id", o.name, ref.table, ref.name)
		}
	}
}

// TestOuterNullableNodeShape covers the NullableColumn surface: a
// NullableColumn.EqOuter comparing against an outer NullableColumn built with
// OuterNullable produces the same erased marker shape.
func TestOuterNullableNodeShape(t *testing.T) {
	p := orderTagNote.EqOuter(OuterNullable(widgetBio))

	n := p.Render()

	if n.Kind != NBinary || n.Table != "order_tags" || n.Column != "note" || n.Op != Eq {
		t.Fatalf("node = %+v, want NBinary order_tags/note =", n)
	}

	ref, ok := n.Value.(OuterRef[widget, string])
	if !ok {
		t.Fatalf("Value type %T, want OuterRef[widget,string]", n.Value)
	}

	if ref.table != "widgets" || ref.name != "bio" {
		t.Fatalf("outer ref carries table %q column %q, want widgets/bio", ref.table, ref.name)
	}
}

func TestCorrelatedReferenceWithoutEnclosingErrors(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	// Outer() used directly on the outermost query's WHERE has no enclosing
	// query to bind to -- a misuse that must fail closed at execution time.
	_, err := From(widgets).
		Where(widgetName.EqOuter(Outer(widgetID))).
		All(ctx, conn)
	if err == nil {
		t.Fatalf("All succeeded, want a no-enclosing-query error")
	}

	if !strings.Contains(err.Error(), `correlated reference to "widgets"."id" has no enclosing query to bind to`) {
		t.Fatalf("err = %v, want the no-enclosing-query error", err)
	}
}

func TestCorrelatedMismatchedOuterTableErrors(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	// The outer reference names widget_orders (orderID's home table), but the
	// enclosing single-table query is widgets: cross-table correlation is a
	// typed execution-time error, never wrong SQL.
	inner := From(widgetOrders).Where(orderWidgetID.EqOuter(Outer(orderID)))

	_, err := From(widgets).
		Where(NotExists[widget](inner)).
		All(ctx, conn)
	if err == nil {
		t.Fatalf("All succeeded, want a mismatched-table error")
	}

	if !strings.Contains(err.Error(), `does not match the enclosing query's table "widgets"`) {
		t.Fatalf("err = %v, want the mismatched-table error", err)
	}
}

func TestCorrelatedJoinWhereNotExists(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// u1 has no profile while u2 does. The correlated NOT EXISTS references
	// the join's LEFT table (join_users) from a subquery over join_profiles,
	// a table that is NOT one side of the join -- so a correct result can
	// only come from the marker actually binding the enclosing join table.
	inner := From(joinProfiles).Where(joinProfileUser.EqOuter(Outer(joinUserID)))

	got, err := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		Where(NotExists[joinUser](inner)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (u1's three orders; u1 has no profile)", len(got))
	}

	for _, r := range got {
		if r.A.ID != "u1" {
			t.Fatalf("row A.ID = %q, want u1", r.A.ID)
		}
	}
}

func TestCorrelatedJoinWhereRightNotExists(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// The marker references the join's RIGHT table (join_orders) from a
	// WhereRight subquery over join_order_items. Only o3 has no items, so
	// only its joined row survives.
	inner := From(joinOrderItems).Where(joinItemOrderID.EqOuter(Outer(joinOrderID)))

	got, err := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		WhereRight(NotExists[joinOrder](inner)).
		OrderByRight(joinOrderID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (only o3 has no items)", len(got))
	}

	if got[0].B.ID != "o3" {
		t.Fatalf("B.ID = %q, want o3", got[0].B.ID)
	}
}

func TestCorrelatedJoinWhereInCorrelated(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// A correlated IN-subquery on the LEFT side: u1 has an order under 150
	// (o1 = 100), so every u1 joined row survives.
	cheapUserIDs := From(joinOrders).
		Where(And(joinOrderCents.Lt(150), joinOrderUser.EqOuter(Outer(joinUserID)))).
		Columns(joinOrderUser.Col())

	got, err := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		Where(joinUserID.InSub(cheapUserIDs)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (u1's orders)", len(got))
	}
}

func TestCorrelatedJoinWhereRightScalar(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// Scalar correlation on the RIGHT side: keep orders whose amount is not
	// this user's maximum order amount. u1's max is 300, so o3 is dropped
	// and o1/o2 survive.
	userMax := From(joinOrders).
		Where(joinOrderUser.EqOuter(Outer(joinUserID))).
		OrderBy(joinOrderCents.Desc()).
		Limit(1).
		Columns(joinOrderCents.Col())

	got, err := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		WhereRight(joinOrderCents.NeqScalar(userMax)).
		OrderByRight(joinOrderID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (o1/o2; o3 equals the max)", len(got))
	}

	if got[0].B.ID != "o1" || got[1].B.ID != "o2" {
		t.Fatalf("order ids = %q/%q, want o1/o2", got[0].B.ID, got[1].B.ID)
	}
}

func TestCorrelatedJoin3WhereC(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// Markers inside a three-table join resolve against its full table set:
	// here the C-side predicate's subquery references the A table.
	inner := From(joinProfiles).Where(joinProfileUser.EqOuter(Outer(joinUserID)))

	got, err := JoinOn3(From(joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		WhereC(NotExists[joinOrderItem](inner)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (u1 has 3 order-item rows)", len(got))
	}
}

func TestCorrelatedJoin3WhereRight(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// B-side predicate correlated to the C table: only o1 has a "pen" item.
	inner := From(joinOrderItems).
		Where(And(joinItemOrderID.EqOuter(Outer(joinOrderID)), joinItemSKU.Eq("pen")))

	got, err := JoinOn3(From(joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		WhereRight(Exists[joinOrder](inner)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (o1's two items)", len(got))
	}

	for _, r := range got {
		if r.B.ID != "o1" {
			t.Fatalf("row B.ID = %q, want o1", r.B.ID)
		}
	}
}

func TestCorrelatedJoin3Where(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// A-side correlated IN-subquery inside a Join3.
	cheapUserIDs := From(joinOrders).
		Where(And(joinOrderCents.Lt(150), joinOrderUser.EqOuter(Outer(joinUserID)))).
		Columns(joinOrderUser.Col())

	got, err := JoinOn3(From(joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		Where(joinUserID.InSub(cheapUserIDs)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
}

func TestCorrelatedJoinMismatchedTableErrors(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// The marker names join_order_items, which is neither side of the
	// users/orders join: a table-set mismatch, never silently-wrong SQL.
	inner := From(joinProfiles).Where(joinProfileUser.EqOuter(Outer(joinItemSKU)))

	j := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		Where(NotExists[joinUser](inner))

	_, err := j.All(ctx, conn)
	if err == nil {
		t.Fatalf("All succeeded, want a mismatched-table error")
	}

	if !strings.Contains(err.Error(), `does not match any of the enclosing query's tables`) {
		t.Fatalf("err = %v, want the table-set mismatch error", err)
	}
}

func TestCorrelatedJoinWithoutEnclosingErrors(t *testing.T) {
	ctx, conn := newJoinDB(t)

	// A marker used directly in the join's own WHERE (not nested inside a
	// subquery predicate) has no enclosing statement: a typed misuse.
	j := JoinOn(From(joinUsers), userOrdersRel, InnerJoin).
		Where(joinUserID.EqOuter(Outer(joinUserID)))

	_, err := j.All(ctx, conn)
	if err == nil {
		t.Fatalf("All succeeded, want a no-enclosing-query error")
	}

	if !strings.Contains(err.Error(), "has no enclosing query to bind to") {
		t.Fatalf("err = %v, want the no-enclosing-query error", err)
	}
}
