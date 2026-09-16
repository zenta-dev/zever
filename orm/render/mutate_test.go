package render

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// fakePostgres is a minimal dialect.Dialect stand-in matching Postgres's
// syntax (numbered $N placeholders, double-quoted identifiers) for tests
// that need Postgres-shaped output without importing the
// orm/dialect/postgres package.
type fakePostgres struct{}

func (fakePostgres) Name() string { return "postgres" }
func (fakePostgres) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}
func (fakePostgres) QuoteIdent(s string) string { return `"` + s + `"` }

func TestInsert(t *testing.T) {
	t.Parallel()
	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := Insert(fakePostgres{}, "users", []string{"id", "email"}, []any{"1", "a@example.com"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES ($1, $2)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		q, _, err := Insert(sqlite.New(), "users", []string{"id", "email"}, []any{"1", "a@example.com"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES (?, ?)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("no columns", func(t *testing.T) {
		q, args, err := Insert(fakePostgres{}, "users", nil, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" DEFAULT VALUES`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})

	t.Run("more columns than values", func(t *testing.T) {
		q, args, err := Insert(fakePostgres{}, "users", []string{"id", "email"}, []any{"1"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id") VALUES ($1)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if len(args) != 1 {
			t.Fatalf("args = %#v, want one element", args)
		}
	})
}

func TestInsertMany(t *testing.T) {
	t.Parallel()
	t.Run("postgres two rows", func(t *testing.T) {
		t.Parallel()
		q, args, err := InsertMany(fakePostgres{}, "users", []string{"id", "email"}, [][]any{
			{"1", "a@example.com"},
			{"2", "b@example.com"},
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES ($1, $2), ($3, $4)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com", "2", "b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite two rows", func(t *testing.T) {
		q, args, err := InsertMany(sqlite.New(), "users", []string{"id", "email"}, [][]any{
			{"1", "a@example.com"},
			{"2", "b@example.com"},
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES (?, ?), (?, ?)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com", "2", "b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no rows", func(t *testing.T) {
		q, args, err := InsertMany(fakePostgres{}, "users", []string{"id", "email"}, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" DEFAULT VALUES`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})

	t.Run("no columns", func(t *testing.T) {
		q, args, err := InsertMany(fakePostgres{}, "users", nil, [][]any{{"1"}})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" DEFAULT VALUES`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})

	t.Run("ragged row is a rendering error, not shifted columns", func(t *testing.T) {
		_, _, err := InsertMany(fakePostgres{}, "users", []string{"id", "email"}, [][]any{
			{"1", "a@example.com"},
			{"2"},
		})
		if err == nil {
			t.Fatalf("err = nil, want a ragged-row error")
		}

		if !strings.Contains(err.Error(), "orm/render: InsertMany") {
			t.Fatalf("err = %v, want it to mention InsertMany", err)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}

	t.Run("assignments render in caller-declaration order", func(t *testing.T) {
		t.Parallel()
		q, args, err := Update(fakePostgres{}, "users",
			[]Assignment{
				{Column: "active", Value: true},
				{Column: "email", Value: "b@example.com"},
			}, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "active" = $1, "email" = $2 WHERE "id" = $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, "b@example.com", "1"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("SetNull-style nil value binds a real NULL parameter", func(t *testing.T) {
		q, args, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "bio", Value: nil}}, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "bio" = $1 WHERE "id" = $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{nil, "1"}) {
			t.Fatalf("args = %#v, want [nil \"1\"]", args)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		q, args, err := Update(sqlite.New(), "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = ? WHERE "id" = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com", "1"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("unset where updates every row", func(t *testing.T) {
		q, _, err := Update(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "x"}}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:     KindCompound,
		Compound: CompoundAnd,
		Children: []Node{
			{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"},
			{Kind: KindBinary, Column: "active", Op: OpEq, Value: false},
		},
	}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := Delete(fakePostgres{}, "users", where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" WHERE ("id" = $1 AND "active" = $2)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", false}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite without a filter deletes everything", func(t *testing.T) {
		q, args, err := Delete(sqlite.New(), "users", Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})
}

func TestInsertReturning(t *testing.T) {
	t.Parallel()
	t.Run("postgres single row", func(t *testing.T) {
		t.Parallel()
		q, args, err := InsertReturning(fakePostgres{}, "users",
			[]string{"id", "email"}, []any{"1", "a@example.com"}, []string{"id", "email"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES ($1, $2) RETURNING "id", "email"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("postgres multi row", func(t *testing.T) {
		q, args, err := InsertManyReturning(fakePostgres{}, "users",
			[]string{"id", "email"}, [][]any{{"1", "a@example.com"}, {"2", "b@example.com"}}, []string{"id"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES ($1, $2), ($3, $4) RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com", "2", "b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite default values", func(t *testing.T) {
		q, args, err := InsertReturning(sqlite.New(), "users", nil, nil, []string{"id"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" DEFAULT VALUES RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})

	t.Run("no returning columns is the plain statement", func(t *testing.T) {
		q, _, err := InsertReturning(fakePostgres{}, "users", []string{"id"}, []any{"1"}, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id") VALUES ($1)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestUpdateJoin(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	t.Run("postgres UPDATE...FROM", func(t *testing.T) {
		t.Parallel()
		q, args, err := UpdateJoin(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1 FROM "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com", true}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite UPDATE...FROM", func(t *testing.T) {
		q, args, err := UpdateJoin(sqlite.New(), "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = ? FROM "orders" WHERE "users"."id" = "orders"."user_id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})
}

func TestDeleteJoin(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	t.Run("postgres DELETE...USING", func(t *testing.T) {
		t.Parallel()
		q, args, err := DeleteJoin(fakePostgres{}, "users", joins, InnerJoin, where, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" USING "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true}) {
			t.Fatalf("args = %#v", args)
		}
	})
}

func TestUpdateReturning(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}
	returning := []string{"id", "email"}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := UpdateReturning(fakePostgres{}, "users",
			[]Assignment{
				{Column: "active", Value: true},
				{Column: "email", Value: "b@example.com"},
			}, where, nil, 0, 0, returning)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "active" = $1, "email" = $2 WHERE "id" = $3 RETURNING "id", "email"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, "b@example.com", "1"}) {
			t.Fatalf("args = %#v, want set-then-where, unchanged by RETURNING", args)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		q, args, err := UpdateReturning(sqlite.New(), "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, where, nil, 0, 0, returning)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = ? WHERE "id" = ? RETURNING "id", "email"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com", "1"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no returning columns is the plain statement", func(t *testing.T) {
		q, _, err := UpdateReturning(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "x"}}, where, nil, 0, 0, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1 WHERE "id" = $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestDeleteReturning(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: "1"}
	returning := []string{"id", "email"}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, args, err := DeleteReturning(fakePostgres{}, "users", where, nil, 0, 0, returning)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" WHERE "id" = $1 RETURNING "id", "email"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		q, args, err := DeleteReturning(sqlite.New(), "users", where, nil, 0, 0, returning)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" WHERE "id" = ? RETURNING "id", "email"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no returning columns is the plain statement", func(t *testing.T) {
		q, _, err := DeleteReturning(fakePostgres{}, "users", where, nil, 0, 0, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" WHERE "id" = $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestUpdateJoinReturning(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	t.Run("postgres UPDATE...FROM, returning after FROM", func(t *testing.T) {
		t.Parallel()
		q, args, err := UpdateJoinReturning(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, where, nil, 0, 0, []string{"id"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1 FROM "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $2 RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		// RETURNING appears after FROM/WHERE and binds no arguments, so the
		// Postgres $N numbering is undisturbed ($1 SET, $2 WHERE).
		if !reflect.DeepEqual(args, []any{"b@example.com", true}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("sqlite UPDATE...FROM, no caller where", func(t *testing.T) {
		q, args, err := UpdateJoinReturning(sqlite.New(), "users",
			[]Assignment{{Column: "email", Value: "b@example.com"}}, joins, InnerJoin, Node{}, nil, 0, 0, []string{"id"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = ? FROM "orders" WHERE "users"."id" = "orders"."user_id" RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no returning columns is the plain joined statement", func(t *testing.T) {
		q, _, err := UpdateJoinReturning(fakePostgres{}, "users",
			[]Assignment{{Column: "email", Value: "x"}}, joins, InnerJoin, where, nil, 0, 0, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `UPDATE "users" SET "email" = $1 FROM "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestDeleteJoinReturning(t *testing.T) {
	t.Parallel()
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}
	where := Node{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true}

	t.Run("postgres DELETE...USING with returning", func(t *testing.T) {
		t.Parallel()
		q, args, err := DeleteJoinReturning(fakePostgres{}, "users", joins, InnerJoin, where, nil, 0, 0, []string{"id"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" USING "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $1 RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("no returning columns is the plain joined statement", func(t *testing.T) {
		q, _, err := DeleteJoinReturning(fakePostgres{}, "users", joins, InnerJoin, where, nil, 0, 0, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `DELETE FROM "users" USING "orders" WHERE "users"."id" = "orders"."user_id" AND "users"."active" = $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestMutationErrorPaths(t *testing.T) {
	t.Parallel()

	badWhere := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "1"}
	badOrder := []OrderTerm{{Func: &FuncExpr{}}}
	sets := []Assignment{{Column: "active", Value: true}}
	joins := []JoinSpec{{ParentCol: "id", ChildTable: "orders", ChildCol: "user_id"}}

	tests := []struct {
		name string
		fn   func() (string, []any, error)
	}{
		{"update bad where", func() (string, []any, error) { return Update(fakePostgres{}, "users", sets, badWhere, nil, 0, 0) }},
		{"update bad order", func() (string, []any, error) { return Update(fakePostgres{}, "users", sets, Node{}, badOrder, 0, 0) }},
		{"delete bad where", func() (string, []any, error) { return Delete(fakePostgres{}, "users", badWhere, nil, 0, 0) }},
		{"delete bad order", func() (string, []any, error) { return Delete(fakePostgres{}, "users", Node{}, badOrder, 0, 0) }},
		{"update returning bad where", func() (string, []any, error) {
			return UpdateReturning(fakePostgres{}, "users", sets, badWhere, nil, 0, 0, []string{"id"})
		}},
		{"delete returning bad where", func() (string, []any, error) {
			return DeleteReturning(fakePostgres{}, "users", badWhere, nil, 0, 0, []string{"id"})
		}},
		{"update join bad where", func() (string, []any, error) {
			return UpdateJoin(fakePostgres{}, "users", sets, joins, InnerJoin, badWhere, nil, 0, 0)
		}},
		{"update join bad order", func() (string, []any, error) {
			return UpdateJoin(fakePostgres{}, "users", sets, joins, InnerJoin, Node{}, badOrder, 0, 0)
		}},
		{"delete join bad where", func() (string, []any, error) {
			return DeleteJoin(fakePostgres{}, "users", joins, InnerJoin, badWhere, nil, 0, 0)
		}},
		{"delete join bad order", func() (string, []any, error) {
			return DeleteJoin(fakePostgres{}, "users", joins, InnerJoin, Node{}, badOrder, 0, 0)
		}},
		{"update join returning bad where", func() (string, []any, error) {
			return UpdateJoinReturning(fakePostgres{}, "users", sets, joins, InnerJoin, badWhere, nil, 0, 0, []string{"id"})
		}},
		{"delete join returning bad where", func() (string, []any, error) {
			return DeleteJoinReturning(fakePostgres{}, "users", joins, InnerJoin, badWhere, nil, 0, 0, []string{"id"})
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, _, err := tc.fn(); err == nil {
				t.Fatal("err = nil, want an error")
			}
		})
	}
}

func TestInsertManyReturningShapes(t *testing.T) {
	t.Parallel()

	rows := [][]any{{"1", "a@example.com"}, {"2", "b@example.com"}}

	t.Run("empty returning renders plain insert many", func(t *testing.T) {
		t.Parallel()

		q, args, err := InsertManyReturning(fakePostgres{}, "users", []string{"id", "email"}, rows, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "users" ("id", "email") VALUES ($1, $2), ($3, $4)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"1", "a@example.com", "2", "b@example.com"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("row width mismatch errors", func(t *testing.T) {
		t.Parallel()

		_, _, err := InsertManyReturning(fakePostgres{}, "users", []string{"id", "email"}, [][]any{{"1"}}, nil)
		if err == nil {
			t.Fatal("err = nil, want a width-mismatch error")
		}
	})
}
