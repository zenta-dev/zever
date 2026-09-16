package orm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// TestColumnEqAnyRendering pins the public Column.EqAny surface end to end:
// the array list is bound as one placeholder per element, with Postgres
// $N numbering, and the quantifier renders `= ANY(ARRAY[...])`.
func TestColumnEqAnyRendering(t *testing.T) {
	ctx := context.Background()

	rec := &ormRecordingExec{dialectName: "postgres"}

	if _, err := From(widgets).Where(widgetQty.EqAny(1, 2, 3)).All(ctx, rec); err != nil {
		t.Fatalf("All: %v", err)
	}

	q, args := rec.last()

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "quantity" = ANY(ARRAY[$1, $2, $3])`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(1), int64(2), int64(3)}) {
		t.Fatalf("args = %#v, want [1 2 3]", args)
	}
}

// TestNullableColumnNeqAllRendering covers the NullableColumn variant and
// the `<> ALL(ARRAY[...])` quantifier.
func TestNullableColumnNeqAllRendering(t *testing.T) {
	ctx := context.Background()

	rec := &ormRecordingExec{dialectName: "postgres"}

	if _, err := From(widgets).Where(widgetBio.NeqAll("a", "b")).All(ctx, rec); err != nil {
		t.Fatalf("All: %v", err)
	}

	q, args := rec.last()

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "bio" <> ALL(ARRAY[$1, $2])`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"a", "b"}) {
		t.Fatalf("args = %#v, want [a b]", args)
	}
}

// TestArrayPredicateEmptyListRendering proves an empty value list renders a
// constant boolean, never the invalid ARRAY[] literal.
func TestArrayPredicateEmptyListRendering(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		pred Predicate[widget]
		want string
	}{
		{"EqAny", widgetQty.EqAny(), `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE 1 = 0`},
		{"NeqAny", widgetQty.NeqAny(), `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE 1 = 0`},
		{"EqAll", widgetQty.EqAll(), `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE 1 = 1`},
		{"NeqAll", widgetQty.NeqAll(), `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE 1 = 1`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &ormRecordingExec{dialectName: "postgres"}

			if _, err := From(widgets).Where(tc.pred).All(ctx, rec); err != nil {
				t.Fatalf("All: %v", err)
			}

			q, args := rec.last()
			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if len(args) != 0 {
				t.Fatalf("args = %#v, want none", args)
			}
		})
	}
}

// TestArrayPredicateCapabilityGate drives the public API through SQLite,
// asserting the typed dialect.ErrUnsupportedByDialect rather than
// silently-wrong SQL.
func TestArrayPredicateCapabilityGate(t *testing.T) {
	ctx := context.Background()

	rec := &ormRecordingExec{dialectName: "sqlite"}

	_, err := From(widgets).Where(widgetQty.EqAny(1)).All(ctx, rec)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}
