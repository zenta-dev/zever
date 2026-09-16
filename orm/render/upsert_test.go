package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func TestInsertOnConflict(t *testing.T) {
	t.Parallel()
	t.Run("postgres do update returning", func(t *testing.T) {
		t.Parallel()
		q, args := InsertOnConflict(postgres.New(), "widgets",
			[]string{"id", "name", "quantity"}, []any{"w1", "Alpha", int64(10)},
			[]string{"id"},
			[]Assignment{{Column: "quantity", Value: int64(99)}, {Column: "name", Value: "Alpha2"}},
			[]string{"id", "name", "quantity"})

		want := `INSERT INTO "widgets" ("id", "name", "quantity") VALUES ($1, $2, $3) ON CONFLICT ("id") DO UPDATE SET "quantity" = $4, "name" = $5 RETURNING "id", "name", "quantity"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		// Argument order must be values-then-conflict-assignments, matching
		// the $1..$5 numbering above.
		if !reflect.DeepEqual(args, []any{"w1", "Alpha", int64(10), int64(99), "Alpha2"}) {
			t.Fatalf("args = %#v, want values-then-assignments", args)
		}
	})

	t.Run("postgres do nothing", func(t *testing.T) {
		q, args := InsertOnConflict(postgres.New(), "widgets",
			[]string{"id", "name"}, []any{"w1", "Alpha"},
			[]string{"id"}, nil, nil)

		want := `INSERT INTO "widgets" ("id", "name") VALUES ($1, $2) ON CONFLICT ("id") DO NOTHING`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "Alpha"}) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("postgres no target do update", func(t *testing.T) {
		q, _ := InsertOnConflict(postgres.New(), "widgets",
			[]string{"id"}, []any{"w1"},
			nil, []Assignment{{Column: "quantity", Value: int64(1)}}, nil)

		want := `INSERT INTO "widgets" ("id") VALUES ($1) ON CONFLICT DO UPDATE SET "quantity" = $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("sqlite placeholders", func(t *testing.T) {
		q, args := InsertOnConflict(sqlite.New(), "widgets",
			[]string{"id", "name"}, []any{"w1", "Alpha"},
			[]string{"id"},
			[]Assignment{{Column: "name", Value: "Beta"}},
			[]string{"id"})

		want := `INSERT INTO "widgets" ("id", "name") VALUES (?, ?) ON CONFLICT ("id") DO UPDATE SET "name" = ? RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "Alpha", "Beta"}) {
			t.Fatalf("args = %#v", args)
		}
	})
}

func TestInsertManyOnConflict(t *testing.T) {
	t.Parallel()
	t.Run("postgres two rows, placeholder numbering continues across rows and assignments", func(t *testing.T) {
		t.Parallel()
		q, args := InsertManyOnConflict(postgres.New(), "widgets",
			[]string{"id", "name"}, [][]any{{"w1", "Alpha"}, {"w2", "Beta"}},
			[]string{"id"},
			[]Assignment{{Column: "quantity", Value: int64(5)}},
			[]string{"id"})

		want := `INSERT INTO "widgets" ("id", "name") VALUES ($1, $2), ($3, $4) ON CONFLICT ("id") DO UPDATE SET "quantity" = $5 RETURNING "id"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "Alpha", "w2", "Beta", int64(5)}) {
			t.Fatalf("args = %#v, want row-major values then assignments", args)
		}
	})

	t.Run("sqlite multi-row do nothing", func(t *testing.T) {
		q, args := InsertManyOnConflict(sqlite.New(), "widgets",
			[]string{"id"}, [][]any{{"w1"}, {"w2"}},
			[]string{"id"}, nil, nil)

		want := `INSERT INTO "widgets" ("id") VALUES (?), (?) ON CONFLICT ("id") DO NOTHING`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "w2"}) {
			t.Fatalf("args = %#v", args)
		}
	})
}
