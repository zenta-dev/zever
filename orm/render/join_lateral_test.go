package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// The lateral-join render tests mirror what orm's LateralJoin2/
// LeftLateralJoin2 builders hand SelectLateral: a left table plus an erased
// inner Subquery whose WHERE carries a correlated OuterRef marker. render
// itself does NOT gate LATERAL (the capability gate lives in orm); these
// tests pin the emitted SQL and the placeholder/arg ordering.

// lateralInnerShape is the common inner subquery: orders whose user_id
// correlates to the enclosing users table, newest first, top 2.
func lateralInnerShape() Subquery {
	return Subquery{
		Table:   "orders",
		Columns: []string{"id", "user_id", "amount"},
		Where: Node{
			Kind:   KindBinary,
			Table:  "orders",
			Column: "user_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "users", Column: "id"},
		},
		Order: []OrderTerm{{Column: "amount", Desc: true}},
		Limit: 2,
	}
}

func TestRenderCrossLateral(t *testing.T) {
	t.Parallel()
	q, args, err := SelectLateral(
		fakePostgres{}, true,
		"users", []string{"id", "name"},
		lateralInnerShape(),
		"orders",
		Node{},
		nil, nil,
		0, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "users"."id", "users"."name", "orders"."id", "orders"."user_id", "orders"."amount" ` +
		`FROM "users" CROSS JOIN LATERAL ` +
		`(SELECT "id", "user_id", "amount" FROM "orders" WHERE "user_id" = "users"."id" ORDER BY "amount" DESC LIMIT $1) ` +
		`AS "orders"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{2}) {
		t.Fatalf("args = %#v, want [2] (the inner LIMIT only; the marker binds nothing)", args)
	}
}

func TestRenderLeftLateralOnTrue(t *testing.T) {
	t.Parallel()
	q, args, err := SelectLateral(
		fakePostgres{}, false,
		"users", []string{"id"},
		Subquery{Table: "orders", Columns: []string{"id"}, Where: Node{
			Kind:   KindBinary,
			Table:  "orders",
			Column: "user_id",
			Op:     OpEq,
			Value:  OuterRef{Table: "users", Column: "id"},
		}},
		"orders",
		Node{},
		nil, nil,
		0, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "users"."id", "orders"."id" FROM "users" LEFT JOIN LATERAL ` +
		`(SELECT "id" FROM "orders" WHERE "user_id" = "users"."id") AS "orders" ON TRUE`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRenderLateralArgOrder(t *testing.T) {
	t.Parallel()
	// The inner subquery's bound args are numbered before the outer WHERE's
	// because the join clause precedes WHERE in the emitted SQL text -- the
	// only ordering that keeps positional `?` dialects (MySQL) correct.
	// Ordering: inner amount > $1, inner LIMIT $2, outer name = $3.
	inner := Subquery{
		Table:   "orders",
		Columns: []string{"id"},
		Where: nCompound(CompoundAnd,
			Node{Kind: KindBinary, Table: "orders", Column: "user_id", Op: OpEq, Value: OuterRef{Table: "users", Column: "id"}},
			nBinary("orders", "amount", OpGt, int64(100))),
		Limit: 2,
	}

	outer := nBinary("users", "name", OpEq, "Alpha")

	q, args, err := SelectLateral(
		fakePostgres{}, true,
		"users", []string{"id"},
		inner,
		"orders",
		outer,
		nil, nil,
		5, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "users"."id", "orders"."id" FROM "users" CROSS JOIN LATERAL ` +
		`(SELECT "id" FROM "orders" WHERE ("user_id" = "users"."id" AND "amount" > $1) LIMIT $2) AS "orders" ` +
		`WHERE "users"."name" = $3 LIMIT $4`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(100), 2, "Alpha", 5}) {
		t.Fatalf("args = %#v, want inner args before outer args in text order", args)
	}
}

func TestRenderLateralOrderByBothSides(t *testing.T) {
	t.Parallel()
	// orderLeft qualifies to the left table and orderInner to the derived
	// alias, so an ORDER BY over either side renders unambiguously.
	q, _, err := SelectLateral(
		fakePostgres{}, true,
		"users", []string{"id"},
		Subquery{Table: "orders", Columns: []string{"id", "amount"}},
		"orders",
		Node{},
		[]OrderTerm{{Column: "id"}},
		[]OrderTerm{{Column: "amount", Desc: true}},
		0, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "users"."id", "orders"."id", "orders"."amount" FROM "users" CROSS JOIN LATERAL ` +
		`(SELECT "id", "amount" FROM "orders") AS "orders" ORDER BY "users"."id" ASC, "orders"."amount" DESC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestRenderLateralMismatchedOuterRejected(t *testing.T) {
	t.Parallel()
	// The marker names a table that is not the left table: the existing
	// correlation guard rejects it, never a dangling column.
	inner := Subquery{Table: "orders", Columns: []string{"id"}, Where: Node{
		Kind:   KindBinary,
		Table:  "orders",
		Column: "user_id",
		Op:     OpEq,
		Value:  OuterRef{Table: "wrong_table", Column: "id"},
	}}

	_, _, err := SelectLateral(fakePostgres{}, true, "users", []string{"id"}, inner, "orders", Node{}, nil, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a mismatched-correlation error")
	}

	if !strings.Contains(err.Error(), `correlated reference to "wrong_table"."id"`) {
		t.Fatalf("err = %v, want the mismatched-table error", err)
	}
}

func TestRenderLateralRendersOnSQLite(t *testing.T) {
	t.Parallel()
	// The render layer is deliberately NOT capability-gated -- orm's builder
	// does the gating -- so rendering the same shape with the sqlite dialect
	// still produces well-formed SQL. This is the "correlated top-N via the
	// render layer" check that does not require a LATERAL-capable engine.
	q, args, err := SelectLateral(
		sqlite.New(), true,
		"users", []string{"id"},
		Subquery{Table: "orders", Columns: []string{"id", "amount"}},
		"orders",
		Node{},
		nil, nil,
		0, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "users"."id", "orders"."id", "orders"."amount" FROM "users" CROSS JOIN LATERAL ` +
		`(SELECT "id", "amount" FROM "orders") AS "orders"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}
