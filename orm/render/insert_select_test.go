package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// selSource is the SELECT source shared by the InsertSelect render tests:
// every column of a widgets table, filtered by `quantity > 15`.
func selSource() SelectSource {
	return SelectSource{
		Table:   "widgets",
		Columns: []string{"id", "name"},
		Where:   Node{Kind: KindBinary, Column: "quantity", Op: OpGt, Value: int64(15)},
	}
}

func TestInsertSelect(t *testing.T) {
	t.Parallel()
	t.Run("postgres plain", func(t *testing.T) {
		t.Parallel()
		q, args, err := InsertSelect(postgres.New(), "archive", []string{"id", "name"}, selSource(), nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > $1`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{int64(15)}) {
			t.Fatalf("args = %#v, want [15]", args)
		}
	})

	t.Run("postgres returning", func(t *testing.T) {
		q, _, err := InsertSelect(postgres.New(), "archive", []string{"id", "name"}, selSource(), []string{"id", "name"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > $1 RETURNING "id", "name"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("sqlite placeholders", func(t *testing.T) {
		q, args, err := InsertSelect(sqlite.New(), "archive", []string{"id", "name"}, selSource(), nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{int64(15)}) {
			t.Fatalf("args = %#v, want [15]", args)
		}
	})
}

func TestInsertSelectOnConflict(t *testing.T) {
	t.Parallel()
	t.Run("postgres do update: select args number first, then set args", func(t *testing.T) {
		t.Parallel()
		q, args, err := InsertSelectOnConflict(postgres.New(), "archive", []string{"id", "name"}, selSource(),
			[]string{"id"}, []Assignment{{Column: "name", Value: "Renamed"}}, []string{"id", "name"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		// The SELECT binds $1; the DO UPDATE SET assignment continues at $2.
		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > $1 ON CONFLICT ("id") DO UPDATE SET "name" = $2 RETURNING "id", "name"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{int64(15), "Renamed"}) {
			t.Fatalf("args = %#v, want select-args-then-set-args", args)
		}
	})

	t.Run("postgres do update continues across multiple select args", func(t *testing.T) {
		src := selSource()
		src.Limit = 5

		q, args, err := InsertSelectOnConflict(postgres.New(), "archive", []string{"id", "name"}, src,
			[]string{"id"}, []Assignment{{Column: "name", Value: "Renamed"}}, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		// SELECT emits $1 (WHERE) and $2 (LIMIT); the SET assignment is $3.
		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > $1 LIMIT $2 ON CONFLICT ("id") DO UPDATE SET "name" = $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{int64(15), 5, "Renamed"}) {
			t.Fatalf("args = %#v, want select-args-then-set-args", args)
		}
	})

	t.Run("postgres do nothing", func(t *testing.T) {
		q, args, err := InsertSelectOnConflict(postgres.New(), "archive", []string{"id", "name"}, selSource(),
			[]string{"id"}, nil, nil)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `INSERT INTO "archive" ("id", "name") SELECT "id", "name" FROM "widgets" WHERE "quantity" > $1 ON CONFLICT ("id") DO NOTHING`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{int64(15)}) {
			t.Fatalf("args = %#v, want [15]", args)
		}
	})
}

func TestInsertSelectErrorPaths(t *testing.T) {
	t.Parallel()

	badSrc := func() SelectSource {
		src := selSource()
		src.Where = Node{Kind: KindBinary, Column: "quantity", Op: OpEqAny, Value: int64(15)}

		return src
	}

	t.Run("source error propagates", func(t *testing.T) {
		t.Parallel()

		if _, _, err := InsertSelect(postgres.New(), "archive", []string{"id", "name"}, badSrc(), nil); err == nil {
			t.Fatal("err = nil, want the source SELECT error")
		}
	})

	t.Run("on-conflict source error propagates", func(t *testing.T) {
		t.Parallel()

		if _, _, err := InsertSelectOnConflictWhere(postgres.New(), "archive", []string{"id", "name"}, badSrc(),
			[]string{"id"}, ConflictWhere{}, nil, nil); err == nil {
			t.Fatal("err = nil, want the source SELECT error")
		}
	})

	t.Run("on-conflict predicate error propagates", func(t *testing.T) {
		t.Parallel()

		where := ConflictWhere{Target: Node{Kind: KindBinary, Column: "active", Op: OpEqAny, Value: 1}}

		if _, _, err := InsertSelectOnConflictWhere(postgres.New(), "archive", []string{"id", "name"}, selSource(),
			[]string{"id"}, where, []Assignment{{Column: "name", Value: "x"}}, nil); err == nil {
			t.Fatal("err = nil, want the conflict WHERE error")
		}
	})
}
