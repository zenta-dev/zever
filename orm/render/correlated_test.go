package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// Correlated-reference test shapes mirror what orm's fluent API builds: a
// KindBinary node whose Value is an OuterRef -- the erased marker for a
// reference to a column of the ENCLOSING query (orm.Outer/OuterNullable).
// The marker's Table names the column's real home table; the renderer
// validates it against the enclosing statement's table set (single table for
// a plain SELECT, every FROM table for a join) and qualifies the column with
// it at execution time (see renderScope).

func correlatedOuter() OuterRef {
	return OuterRef{Table: "widgets", Column: "id"}
}

func TestRenderCorrelatedExists(t *testing.T) {
	t.Parallel()
	// The classic anti-join inner: o.widget_id = widgets.id, rendered fully
	// qualified so the inner FROM's own table cannot be confused with it.
	deepWhere := Node{
		Kind:   KindBinary,
		Table:  "widget_orders",
		Column: "widget_id",
		Op:     OpEq,
		Value:  correlatedOuter(),
	}

	where := Node{Kind: KindSubquery, Value: nSub("widget_orders", []string{"id", "widget_id", "amount"}, deepWhere)}

	q, args, err := Select(sqlite.New(), "widgets", []string{"id", "name", "quantity", "bio"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE EXISTS (SELECT "id", "widget_id", "amount" FROM "widget_orders" WHERE "widget_id" = "widgets"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none (a correlated column reference binds no argument)", args)
	}
}

func TestRenderCorrelatedMixedArgOrder(t *testing.T) {
	t.Parallel()
	// Outer bound arg, then a correlated marker (binds nothing), then an
	// inner bound arg: the marker must not disturb $N numbering, so outer
	// args stay $1.. and inner args continue after them in text order.
	innerWhere := nCompound(CompoundAnd,
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  correlatedOuter(),
		},
		nBinary("widget_orders", "id", OpEq, "o2"))

	where := nCompound(CompoundAnd,
		nBinary("widgets", "quantity", OpGte, int64(20)),
		nCompound(CompoundNot, Node{Kind: KindSubquery,
			Value: nSub("widget_orders", []string{"id", "widget_id", "amount"}, innerWhere)}))

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id", "name", "quantity", "bio"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE ("quantity" >= $1 AND NOT (EXISTS (SELECT "id", "widget_id", "amount" FROM "widget_orders" WHERE ("widget_id" = "widgets"."id" AND "id" = $2))))`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(20), "o2"}) {
		t.Fatalf("args = %#v, want outer arg before inner arg, marker binding nothing", args)
	}
}

func TestRenderCorrelatedInSub(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:   KindIn,
		Table:  "widgets",
		Column: "quantity",
		Op:     OpIn,
		Value: nSub("widget_orders", []string{"amount"},
			Node{
				Kind:   KindBinary,
				Table:  "widget_orders",
				Column: "widget_id",
				Op:     OpEq,
				Value:  correlatedOuter(),
			}),
	}

	q, args, err := Select(sqlite.New(), "widgets", []string{"id", "name", "quantity", "bio"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "quantity" IN (SELECT "amount" FROM "widget_orders" WHERE "widget_id" = "widgets"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRenderCorrelatedScalar(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:   KindBinary,
		Table:  "widgets",
		Column: "quantity",
		Op:     OpEq,
		Value: nSub("widget_orders", []string{"amount"},
			nCompound(CompoundAnd,
				Node{
					Kind:   KindBinary,
					Table:  "widget_orders",
					Column: "widget_id",
					Op:     OpEq,
					Value:  correlatedOuter(),
				},
				nBinary("widget_orders", "id", OpEq, "o1"))),
	}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id", "name", "quantity", "bio"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "quantity" = (SELECT "amount" FROM "widget_orders" WHERE ("widget_id" = "widgets"."id" AND "id" = $1))`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"o1"}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

func TestRenderNestedCorrelation(t *testing.T) {
	t.Parallel()
	// A correlated EXISTS whose subquery's own IN-subquery carries a SECOND
	// level of correlation -- the deep marker binds to the MIDDLE subquery's
	// table (widget_orders), the shallow one to the outer widgets.
	deep := nSub("order_tags", []string{"order_id"},
		nCompound(CompoundAnd,
			nBinary("order_tags", "tag", OpEq, "rush"),
			Node{
				Kind:   KindBinary,
				Table:  "order_tags",
				Column: "order_id",
				Op:     OpEq,
				Value:  OuterRef{Table: "widget_orders", Column: "id"},
			}))

	middle := Subquery{
		Table:   "widget_orders",
		Columns: []string{"id", "widget_id", "amount"},
		Where: nCompound(CompoundAnd,
			Node{
				Kind:   KindBinary,
				Table:  "widget_orders",
				Column: "widget_id",
				Op:     OpEq,
				Value:  correlatedOuter(),
			},
			Node{Kind: KindIn, Table: "widget_orders", Column: "id", Op: OpIn, Value: deep}),
	}

	where := Node{Kind: KindSubquery, Value: middle}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id", "name"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name" FROM "widgets" WHERE EXISTS (SELECT "id", "widget_id", "amount" FROM "widget_orders" WHERE ("widget_id" = "widgets"."id" AND "id" IN (SELECT "order_id" FROM "order_tags" WHERE ("tag" = $1 AND "order_id" = "widget_orders"."id"))))`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"rush"}) {
		t.Fatalf("args = %#v, want only the deepest bound value", args)
	}
}

func TestRenderCorrelatedCount(t *testing.T) {
	t.Parallel()
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  correlatedOuter(),
		})

	q, args, err := Count(fakePostgres{}, "widgets", Node{Kind: KindSubquery, Value: inner})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT COUNT(*) FROM "widgets" WHERE EXISTS (SELECT "id" FROM "widget_orders" WHERE "widget_id" = "widgets"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRenderCorrelatedMismatchedTableRejected(t *testing.T) {
	t.Parallel()
	// The marker names orders as its column's home table, but the enclosing
	// single-table SELECT is widgets: a cross-table reference must be a typed
	// rendering error, never silently wrong SQL.
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "orders", Column: "id"},
		})

	_, _, err := Select(fakePostgres{}, "widgets", []string{"id"}, Node{Kind: KindSubquery, Value: inner}, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a mismatched-correlation error")
	}

	if !strings.Contains(err.Error(), `correlated reference to "orders"."id" does not match the enclosing query's table "widgets"`) {
		t.Fatalf("err = %v, want the mismatched-table error", err)
	}
}

func TestRenderCorrelatedWithoutEnclosingRejected(t *testing.T) {
	t.Parallel()
	// A marker used directly in the outermost SELECT's WHERE has no enclosing
	// query; orm's API can build that only as a misuse, and rendering must
	// fail closed rather than emit an unqualified column.
	_, _, err := Select(fakePostgres{}, "widgets", []string{"id"},
		Node{Kind: KindBinary, Table: "widgets", Column: "name", Op: OpEq, Value: correlatedOuter()}, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a no-enclosing-query error")
	}

	if !strings.Contains(err.Error(), "has no enclosing query to bind to") {
		t.Fatalf("err = %v, want the no-enclosing-query error", err)
	}
}

func TestRenderCorrelatedInJoinResolves(t *testing.T) {
	t.Parallel()
	// Correlation across a JOIN is now in scope: a marker inside a subquery
	// under a SelectJoin resolves against the join's table set, rendering the
	// referenced column qualified to its own home table.
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  correlatedOuter(),
		})

	whereLeft := Node{Kind: KindSubquery, Value: inner}

	q, args, err := SelectJoin(sqlite.New(), InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		whereLeft, Node{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widget_orders"."id", "widget_orders"."widget_id" FROM "widgets" INNER JOIN "widget_orders" ON "widgets"."id" = "widget_orders"."widget_id" WHERE EXISTS (SELECT "id" FROM "widget_orders" WHERE "widget_id" = "widgets"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none (a correlated column reference binds no argument)", args)
	}
}

func TestRenderCorrelatedInJoinRightTableResolves(t *testing.T) {
	t.Parallel()
	// The marker names the join's RIGHT table as its home: it must resolve
	// just as the left one does, still rendered qualified to its own table.
	inner := nSub("order_tags", []string{"order_id"},
		Node{
			Kind:   KindBinary,
			Table:  "order_tags",
			Column: "order_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "widget_orders", Column: "id"},
		})

	whereRight := Node{Kind: KindSubquery, Value: inner}

	q, args, err := SelectJoin(sqlite.New(), InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		Node{}, whereRight, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widget_orders"."id", "widget_orders"."widget_id" FROM "widgets" INNER JOIN "widget_orders" ON "widgets"."id" = "widget_orders"."widget_id" WHERE EXISTS (SELECT "order_id" FROM "order_tags" WHERE "order_id" = "widget_orders"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRenderCorrelatedInJoin3Resolves(t *testing.T) {
	t.Parallel()
	// A three-table join's WHERE carries a marker referencing its rightmost
	// table (C), drawn from the join's full table set.
	inner := nSub("d", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "d",
			Column: "a_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "c", Column: "id"},
		})

	whereC := Node{Kind: KindSubquery, Value: inner}

	q, args, err := SelectJoin3(sqlite.New(), InnerJoin,
		"a", []string{"id"},
		"b", []string{"id"},
		"c", []string{"id"},
		"id", "a_id", "id", "b_id",
		Node{}, Node{}, whereC, nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "a"."id", "b"."id", "c"."id" FROM "a" INNER JOIN "b" ON "a"."id" = "b"."a_id" INNER JOIN "c" ON "b"."id" = "c"."b_id" WHERE EXISTS (SELECT "id" FROM "d" WHERE "a_id" = "c"."id")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRenderCorrelatedJoinMismatchedTableRejected(t *testing.T) {
	t.Parallel()
	// The marker names a table that is neither side of the join: the
	// table-set-aware guard must reject it, never render a dangling column.
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "orders", Column: "id"},
		})

	whereLeft := Node{Kind: KindSubquery, Value: inner}

	_, _, err := SelectJoin(fakePostgres{}, InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		whereLeft, Node{}, nil, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a mismatched-correlation error")
	}

	if !strings.Contains(err.Error(), `correlated reference to "orders"."id" does not match any of the enclosing query's tables`) {
		t.Fatalf("err = %v, want the table-set mismatch error", err)
	}
}

func TestRenderCorrelatedJoinWithoutEnclosingRejected(t *testing.T) {
	t.Parallel()
	// A marker used directly in the JOIN's own WHERE has no ENCLOSING
	// statement (the join is the outermost statement): still a typed misuse.
	whereLeft := Node{Kind: KindBinary, Table: "widgets", Column: "name", Op: OpEq, Value: correlatedOuter()}

	_, _, err := SelectJoin(fakePostgres{}, InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		whereLeft, Node{}, nil, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a no-enclosing-query error")
	}

	if !strings.Contains(err.Error(), "has no enclosing query to bind to") {
		t.Fatalf("err = %v, want the no-enclosing-query error", err)
	}
}

func TestRenderCorrelatedShadowedByInnerTableRejected(t *testing.T) {
	t.Parallel()
	// A marker naming a table that is BOTH an enclosing join table and the
	// inner subquery's own FROM table would silently bind to the inner FROM
	// table: reject it rather than emit silently-wrong SQL.
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "widget_orders", Column: "id"},
		})

	whereLeft := Node{Kind: KindSubquery, Value: inner}

	_, _, err := SelectJoin(fakePostgres{}, InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		whereLeft, Node{}, nil, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a shadowed-correlation error")
	}

	if !strings.Contains(err.Error(), `correlated reference to "widget_orders"."id" is shadowed`) {
		t.Fatalf("err = %v, want the shadowed-correlation error", err)
	}
}

func TestRenderCorrelatedJoinPlaceholderOrder(t *testing.T) {
	t.Parallel()
	// An outer bound arg in the join's WHERE is numbered before the inner
	// subquery's bound arg, while the correlated marker binds nothing -- the
	// $N-ordering proof for Postgres.
	innerWhere := nCompound(CompoundAnd,
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  correlatedOuter(),
		},
		nBinary("widget_orders", "id", OpEq, "o2"))

	whereLeft := nCompound(CompoundAnd,
		nBinary("widgets", "quantity", OpGte, int64(20)),
		Node{Kind: KindSubquery, Value: nSub("widget_orders", []string{"id"}, innerWhere)})

	q, args, err := SelectJoin(fakePostgres{}, InnerJoin,
		"widgets", []string{"id", "name"},
		"widget_orders", []string{"id", "widget_id"},
		"id", "widget_id",
		whereLeft, Node{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widget_orders"."id", "widget_orders"."widget_id" FROM "widgets" INNER JOIN "widget_orders" ON "widgets"."id" = "widget_orders"."widget_id" WHERE ("widgets"."quantity" >= $1 AND EXISTS (SELECT "id" FROM "widget_orders" WHERE ("widget_id" = "widgets"."id" AND "id" = $2)))`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(20), "o2"}) {
		t.Fatalf("args = %#v, want the outer arg before the inner arg, marker binding nothing", args)
	}
}

func TestCorrelatedBinaryIsUncacheable(t *testing.T) {
	// A marker's rendered text depends on the enclosing statement's table
	// name, which is not part of the shape fingerprint (and the marker binds
	// no argument, so the cached arg collector must never see it): a binary
	// carrying an OuterRef value bypasses the shape cache, alone or inside a
	// compound.
	n := Node{
		Kind:   KindBinary,
		Table:  "widget_orders",
		Column: "widget_id",
		Op:     OpEq,
		Value:  correlatedOuter(),
	}

	if cacheableWhere(n) {
		t.Fatalf("correlated binary reported cacheable")
	}

	if cacheableWhere(nCompound(CompoundAnd, n, nBinary("widgets", "name", OpEq, "x"))) {
		t.Fatalf("compound containing a correlated binary reported cacheable")
	}
}

func TestCorrelatedShapesBypassCache(t *testing.T) {
	// End-to-end: a correlated EXISTS and a correlated IN-subquery render
	// fresh every call (never a cache hit) while staying byte-identical --
	// the same no-hit contract the non-correlated subquery shapes already
	// enforce.
	inner := nSub("widget_orders", []string{"id"},
		Node{
			Kind:   KindBinary,
			Table:  "widget_orders",
			Column: "widget_id",
			Op:     OpEq,
			Value:  correlatedOuter(),
		})

	renderTwiceNoHit(t, func() (string, []any, error) {
		return Select(sqlite.New(), "widgets", []string{"id"}, Node{Kind: KindSubquery, Value: inner}, nil, 0, 0)
	})

	renderTwiceNoHit(t, func() (string, []any, error) {
		return Select(sqlite.New(), "widgets", []string{"id"},
			Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpIn, Value: inner}, nil, 0, 0)
	})
}
