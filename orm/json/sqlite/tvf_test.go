package sqlite

import (
	"errors"
	"strings"
	"testing"

	orm "github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// docID is the id column of the shared doc fixture (sqlite_test.go); the TVF
// join tests qualify the left table with it.
var docID = orm.NewColumn[doc, string]("docs", "id")

// TestJSONEachSourceRender pins the rendered function call text for the four
// JSONEach/JSONTree constructors: every identifier is double-quoted and the
// optional path is a standard string literal.
func TestJSONEachSourceRender(t *testing.T) {
	tests := []struct {
		name string
		src  Source[EachRow]
		want string
	}{
		{"json_each", JSONEach[EachRow](docData, "je"), `json_each("docs"."data")`},
		{"json_each path", JSONEachPath[EachRow](docData, "$.tags", "je"), `json_each("docs"."data", '$.tags')`},
		{"json_tree", JSONTree[EachRow](docData, "jt"), `json_tree("docs"."data")`},
		{"json_tree path", JSONTreePath[EachRow](docData, "$.a[0]", "jt"), `json_tree("docs"."data", '$.a[0]')`},
		{"path quote escaped", JSONEachPath[EachRow](docData, "$.o'brien", "je"), `json_each("docs"."data", '$.o''brien')`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.src.RenderSource(sqlite.New())
			if err != nil {
				t.Fatalf("RenderSource: %v", err)
			}

			if got != tc.want {
				t.Fatalf("SQL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestJSONEachSourceCapabilityGate asserts the source fails closed with a
// typed dialect.ErrUnsupportedByDialect on Postgres, never
// rendering invalid SQL.
func TestJSONEachSourceCapabilityGate(t *testing.T) {
	src := JSONEach[EachRow](docData, "je")

	if _, err := src.RenderSource(postgres.New()); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("RenderSource on postgres: err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := src.RenderSource(sqlite.New()); err != nil {
		t.Fatalf("RenderSource on sqlite: %v", err)
	}
}

// TestJSONEachEmptyColumnRejected proves an empty document column fails
// closed at render time rather than emitting `json_each("")`.
func TestJSONEachEmptyColumnRejected(t *testing.T) {
	var src Source[EachRow]
	src.fn = "json_each"

	if _, err := src.RenderSource(sqlite.New()); err == nil {
		t.Fatal("RenderSource with empty column = nil error, want an error")
	}
}

// TestJSONEachJoinScans is the real-engine proof that a json_each source is
// usable in a real query: it joins docs to json_each(docs.data, '$.tags'),
// filters by the source's typed `value` handle, orders by the left table, and
// scans each row into EachRow through one rows.Scan per row.
func TestJSONEachJoinScans(t *testing.T) {
	ctx, conn := newDocsDB(t)

	src := JSONEachPath[EachRow](docData, "$.tags", "je")

	got, err := orm.JoinTVF(orm.From(docs), src).
		WhereSource(src.Value.Eq("go")).
		OrderBy(docID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (d1 and d2 both contain go)", len(got))
	}

	for _, r := range got {
		if r.B.Value != "go" || r.B.Type != "text" {
			t.Fatalf("row B = %+v, want value=go type=text", r.B)
		}
	}

	if got[0].A.ID != "d1" || got[1].A.ID != "d2" {
		t.Fatalf("ids = %q, %q; want d1, d2", got[0].A.ID, got[1].A.ID)
	}
}

// TestJSONEachLeftJoinNone proves the LEFT form turns a path that yields no
// rows into Option[B]{}.IsSome() == false, never a zero EachRow.
func TestJSONEachLeftJoinNone(t *testing.T) {
	ctx, conn := newDocsDB(t)

	src := JSONEachPath[EachRow](docData, "$.missing", "je")

	got, err := orm.LeftJoinTVF(orm.From(docs), src).
		Where(docID.Eq("d1")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}

	if got[0].A.ID != "d1" {
		t.Fatalf("A.ID = %q, want d1", got[0].A.ID)
	}

	if got[0].B.IsSome() {
		t.Fatalf("B = %+v, want None (no matching path)", got[0].B)
	}
}

// TestJSONTreeRecurses proves json_tree emits one row per node, so a nested
// document yields the root plus each nested key.
func TestJSONTreeRecurses(t *testing.T) {
	ctx, conn := newDocsDB(t)

	src := JSONTree[TreeRow](docData, "jt")

	got, err := orm.JoinTVF(orm.From(docs), src).Where(docID.Eq("d1")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// d1's document has root, name, role, tags, two array elements, and meta
	// -- json_tree yields at least those nodes.
	if len(got) < 6 {
		t.Fatalf("len(got) = %d, want >= 6 json_tree nodes", len(got))
	}

	var sawTagsPath bool
	for _, r := range got {
		if strings.HasPrefix(r.B.Fullkey, "$.tags") {
			sawTagsPath = true
		}
	}

	if !sawTagsPath {
		t.Fatalf("no row had a $.tags fullkey: %+v", got)
	}
}

// TestJSONEachStream proves the Stream path scans rows through the same
// scanJoinRow machinery.
func TestJSONEachStream(t *testing.T) {
	ctx, conn := newDocsDB(t)

	src := JSONEachPath[EachRow](docData, "$.tags", "je")

	var n int

	for row, err := range orm.JoinTVF(orm.From(docs), src).Where(docID.Eq("d1")).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.B.Value == nil {
			t.Fatalf("row.B.Value = nil, want a value")
		}

		n++
	}

	if n != 2 {
		t.Fatalf("streamed %d rows, want 2", n)
	}
}

// TestJSONEachRowSurface proves EachRow materializes in SrcColumns order:
// all nine json_each output columns scan positionally.
func TestJSONEachRowSurface(t *testing.T) {
	var row EachRow

	err := row.Scan(tvfTestRows{vals: []any{"k", "v", "text", int64(1), int64(2), int64(3), "fk", "p", "r"}})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if row.Key != "k" || row.Value != "v" || row.Type != "text" {
		t.Fatalf("row = %+v, want key/value/type k/v/text", row)
	}
}

// tvfTestRows is a one-shot orm.Row over fixed values for TVF row Scan
// tests (no live database needed). It assigns each value into its
// destination like a driver would: *any verbatim, *string/*int64 by type,
// and single-argument Scanner destinations (Option) through their Scan.
type tvfTestRows struct {
	vals []any
}

func (r tvfTestRows) Scan(dest ...any) error {
	for i, d := range dest {
		if i >= len(r.vals) {
			return errTestRowsExhausted
		}

		v := r.vals[i]

		switch p := d.(type) {
		case *any:
			*p = v
		case interface{ Scan(any) error }:
			if err := p.Scan(v); err != nil {
				return err
			}
		case *string:
			s, ok := v.(string)
			if !ok {
				return errTestRowsType
			}

			*p = s
		case *int64:
			n, ok := v.(int64)
			if !ok {
				return errTestRowsType
			}

			*p = n
		default:
			return errTestRowsType
		}
	}

	return nil
}

var (
	errTestRowsExhausted = testRowsError("row feed exhausted")
	errTestRowsType      = testRowsError("unsupported test value type")
)

type testRowsError string

func (e testRowsError) Error() string { return "tvf test rows: " + string(e) }
