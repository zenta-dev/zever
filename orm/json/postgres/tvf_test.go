package postgres

import (
	"errors"
	"strings"
	"testing"

	orm "github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestArrayElementsSourceRender pins the rendered function call text: the
// document column is table-qualified (the Postgres FROM-clause function is
// implicitly lateral), every identifier double-quoted.
func TestArrayElementsSourceRender(t *testing.T) {
	tests := []struct {
		name string
		src  Source[ElemRow]
		want string
	}{
		{"jsonb", ArrayElements[ElemRow](docData, "e"), `jsonb_array_elements("docs"."data")`},
		{"text", ArrayElementsText[ElemRow](docData, "e"), `jsonb_array_elements_text("docs"."data")`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.src.RenderSource(postgres.New())
			if err != nil {
				t.Fatalf("RenderSource: %v", err)
			}

			if got != tc.want {
				t.Fatalf("SQL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestArrayElementsSourceCapabilityGate asserts the source fails closed with
// a typed dialect.ErrUnsupportedByDialect on SQLite.
func TestArrayElementsSourceCapabilityGate(t *testing.T) {
	src := ArrayElements[ElemRow](docData, "e")

	if _, err := src.RenderSource(sqlite.New()); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("RenderSource on sqlite: err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := src.RenderSource(postgres.New()); err != nil {
		t.Fatalf("RenderSource on postgres: %v", err)
	}
}

// TestArrayElementsJoinRender proves the source composes into a real join
// query: the emitted SQL is a CROSS JOIN over the set-returning function, and
// the source's typed `value` handle becomes a bound predicate qualified to the
// derived-table alias.
func TestArrayElementsJoinRender(t *testing.T) {
	ctx := t.Context()

	src := ArrayElements[ElemRow](docData, "e")

	conn := &captureExec{}

	_, err := orm.JoinTVF(orm.From(docs), src).
		WhereSource(src.Value.Eq("go")).
		OrderBy(orm.NewColumn[doc, string]("docs", "id").Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "docs"."id", "docs"."data", "e"."value" FROM "docs" ` +
		`CROSS JOIN jsonb_array_elements("docs"."data") AS "e" ` +
		`WHERE "e"."value" = $1 ORDER BY "docs"."id" ASC`
	if conn.lastQuery != want {
		t.Fatalf("query = %q, want %q", conn.lastQuery, want)
	}

	if len(conn.lastArgs) != 1 || conn.lastArgs[0] != "go" {
		t.Fatalf("args = %#v, want [go]", conn.lastArgs)
	}
}

// TestArrayElementsLeftJoinRender pins the null-safe LEFT JOIN ... ON TRUE
// shape.
func TestArrayElementsLeftJoinRender(t *testing.T) {
	ctx := t.Context()

	src := ArrayElements[ElemRow](docData, "e")

	conn := &captureExec{}

	_, err := orm.LeftJoinTVF(orm.From(docs), src).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "docs"."id", "docs"."data", "e"."value" FROM "docs" ` +
		`LEFT JOIN jsonb_array_elements("docs"."data") AS "e" ON TRUE`
	if conn.lastQuery != want {
		t.Fatalf("query = %q, want %q", conn.lastQuery, want)
	}

	if !strings.Contains(conn.lastQuery, "ON TRUE") {
		t.Fatalf("query = %q, want LEFT JOIN ... ON TRUE", conn.lastQuery)
	}
}

// TestArrayElementsRowSurface proves the source's row type materializes:
// NewRow returns a zero ElemRow and Scan reads the single value column.
func TestArrayElementsRowSurface(t *testing.T) {
	var src Source[ElemRow]

	row := src.NewRow()
	if row.Value != nil {
		t.Fatalf("NewRow().Value = %#v, want nil", row.Value)
	}

	if err := row.Scan(tvfTestRows{vals: []any{"go"}}); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if row.Value != "go" {
		t.Fatalf("Value = %#v, want %q", row.Value, "go")
	}

	empty := Source[ElemRow]{fn: "jsonb_array_elements"}

	if _, err := empty.RenderSource(postgres.New()); err == nil {
		t.Fatal("RenderSource with an empty document column succeeded, want an error")
	}
}

// tvfTestRows is a one-shot orm.Row over fixed values for TVF row Scan
// tests (no live Postgres needed).
type tvfTestRows struct {
	vals []any
}

func (r tvfTestRows) Scan(dest ...any) error {
	for i, d := range dest {
		if p, ok := d.(*any); ok && i < len(r.vals) {
			*p = r.vals[i]
		}
	}

	return nil
}
