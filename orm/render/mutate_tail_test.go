package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestUpdateOrderLimitOffset pins the ORDER BY / LIMIT / OFFSET tail on a
// plain UPDATE: WHERE → ORDER BY → LIMIT → OFFSET, with the Postgres $N
// numbering continuing through the bound limit/offset placeholders. The
// tail renders for ANY dialect shape (render is capability-agnostic; the
// orm layer rejects unsupported dialect/statement combos before render).
func TestUpdateOrderLimitOffset(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}
	order := []OrderTerm{{Column: "name", Desc: true}, {Column: "id"}}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "active", Value: true}}, where, order, 10, 20)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "active" = $1 WHERE "id" = $2 ORDER BY "name" DESC, "id" ASC LIMIT $3 OFFSET $4`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, "1", 10, 20}) {
			t.Fatalf("args = %#v, want set-then-where-then-limit-offset", args)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		q, args, err := Update(sqlite.New(), "users",
			[]Assignment{{Column: "active", Value: true}}, where, order, 10, 5)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "active" = ? WHERE "id" = ? ORDER BY "name" DESC, "id" ASC LIMIT ? OFFSET ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, "1", 10, 5}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("order without where", func(t *testing.T) {
		q, args, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "active", Value: true}}, Node{}, order, 10, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "active" = $1 ORDER BY "name" DESC, "id" ASC LIMIT $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, 10}) {
			t.Fatalf("args = %#v", args)
		}
	})

	// Zero and negative limit/offset mean "no clause", exactly like Select:
	// a limit of 0 never renders LIMIT, and the statement is plain.
	t.Run("zero and negative limit offset leave the statement plain", func(t *testing.T) {
		q, args, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "active", Value: true}}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `UPDATE "users" SET "active" = $1`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true}) {
			t.Fatalf("args = %#v", args)
		}

		q, _, err = Update(fakePostgres{}, "users",
			[]Assignment{{Column: "active", Value: true}}, Node{}, nil, -1, -5)
		if err != nil {
			t.Fatalf("err (negative) = %v, want nil", err)
		}

		if want := `UPDATE "users" SET "active" = $1`; q != want {
			t.Fatalf("query (negative) = %q, want %q", q, want)
		}
	})

	// Positive limit with zero offset renders LIMIT but no OFFSET.
	t.Run("limit without offset", func(t *testing.T) {
		q, _, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "active", Value: true}}, Node{}, nil, 7, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `UPDATE "users" SET "active" = $1 LIMIT $2`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestDeleteOrderLimitOffset(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}
	order := []OrderTerm{{Column: "id", Desc: true}}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := Delete(fakePostgres{}, "users", where, order, 5, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" WHERE "id" = $1 ORDER BY "id" DESC LIMIT $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", 5}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no where", func(t *testing.T) {
		q, args, err := Delete(fakePostgres{}, "users", Node{}, order, 3, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" ORDER BY "id" DESC LIMIT $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{3}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("zero limit leaves the delete plain", func(t *testing.T) {
		q, _, err := Delete(fakePostgres{}, "users", Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `DELETE FROM "users"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

// TestUpdateReturningOrderLimitOffset pins the full clause order on an
// UPDATE ... RETURNING: RETURNING always comes last, after the ORDER BY /
// LIMIT / OFFSET tail, and binds nothing (the $N numbering is unaffected).
func TestUpdateReturningOrderLimitOffset(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}

	q, args, err := UpdateReturning(fakePostgres{}, "users",
		[]Assignment{{Column: "active", Value: true}},
		where, []OrderTerm{{Column: "name", Desc: true}}, 10, 20,
		[]string{"id", "name"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `UPDATE "users" SET "active" = $1 WHERE "id" = $2 ORDER BY "name" DESC LIMIT $3 OFFSET $4 RETURNING "id", "name"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{true, "1", 10, 20}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestDeleteReturningOrderLimitOffset(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}

	q, args, err := DeleteReturning(fakePostgres{}, "users",
		where, []OrderTerm{{Column: "id"}}, 5, 0, []string{"id"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `DELETE FROM "users" WHERE "id" = $1 ORDER BY "id" ASC LIMIT $2 RETURNING "id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"1", 5}) {
		t.Fatalf("args = %#v", args)
	}
}

// TestUpdateJoinOrderLimitOffset pins the tail on a joined UPDATE. The
// caller's where AND'ed join conditions precede ORDER BY / LIMIT / OFFSET.
func TestUpdateJoinOrderLimitOffset(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}
	order := []OrderTerm{{Column: "users.id"}}

	t.Run("postgres UPDATE...FROM", func(t *testing.T) {
		t.Parallel()
		q, args, err := UpdateJoin(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, where, order, 10, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1 FROM "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $2 ORDER BY "users"."id" ASC LIMIT $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com", true, 10}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite UPDATE...FROM", func(t *testing.T) {
		q, args, err := UpdateJoin(sqlite.New(), "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, Node{}, order, 10, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = ? FROM "orders" WHERE "users"."id" = "orders"."user_id" ORDER BY "users"."id" ASC LIMIT ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com", 10}) {
			t.Fatalf("args = %#v", args)
		}
	})
}

func TestDeleteJoinOrderLimitOffset(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}
	order := []OrderTerm{{Column: "users.id"}}

	t.Run("postgres DELETE...USING", func(t *testing.T) {
		t.Parallel()
		q, args, err := DeleteJoin(fakePostgres{}, "users", joins, InnerJoin, where, order, 10, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" USING "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $1 ORDER BY "users"."id" ASC LIMIT $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, 10}) {
			t.Fatalf("args = %#v", args)
		}
	})
}

// TestUpdateJoinReturningOrderLimitOffset pins RETURNING after the
// ORDER/LIMIT tail on a joined UPDATE.
func TestUpdateJoinReturningOrderLimitOffset(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	q, args, err := UpdateJoinReturning(fakePostgres{}, "users",
		[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, where,
		[]OrderTerm{{Column: "users.id"}}, 10, 0, []string{"id"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `UPDATE "users" SET "email" = $1 FROM "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $2 ORDER BY "users"."id" ASC LIMIT $3 RETURNING "id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"b@example.com", true, 10}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestDeleteJoinReturningOrderLimitOffset(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	q, args, err := DeleteJoinReturning(fakePostgres{}, "users", joins, InnerJoin, where,
		[]OrderTerm{{Column: "users.id"}}, 10, 0, []string{"id"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `DELETE FROM "users" USING "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $1 ORDER BY "users"."id" ASC LIMIT $2 RETURNING "id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{true, 10}) {
		t.Fatalf("args = %#v", args)
	}
}
