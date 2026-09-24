package sqlite

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	orm "github.com/zenta-dev/zever/orm"
)

// doc is a small fixture entity with a JSON-text column, mirroring the
// shape schema codegen generates for a `data: string` field
// that stores JSON text.
type doc struct {
	ID   string
	Data string
}

func (d *doc) Scan(row orm.Row) error {
	return row.Scan(&d.ID, &d.Data)
}

var (
	docs    = orm.NewTable[doc]("docs", []string{"id", "data"})
	docData = orm.NewColumn[doc, string]("docs", "data")
)

// newDocsDB opens an in-memory sqlite database seeded with three JSON
// documents, proving the json1 round-trip end to end.
func newDocsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err := conn.Exec(ctx, `CREATE TABLE docs (id text, data text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct{ id, data string }{
		{"d1", `{"name":"alice","role":"admin","tags":["go","json"],"meta":{"active":true}}`},
		{"d2", `{"name":"bob","role":"user","tags":["go","sql"],"meta":{"active":false}}`},
		{"d3", `{"name":"carol","role":"user","tags":["python"],"meta":{}}`},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO docs (id, data) VALUES (?, ?)`, r.id, r.data); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}

func TestSQLiteJSONExtractText(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(ExtractText(docData, "name").Eq("bob")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d2" {
		t.Fatalf("ExtractText(name=bob) = %v, want exactly d2", ids(got))
	}
}

func TestSQLiteJSONChainedExtract(t *testing.T) {
	ctx, conn := newDocsDB(t)

	// Chained extraction: json_extract(data, '$.tags[0]') -- step into the
	// object, then index the array.
	got, err := orm.From(docs).
		Where(JSONPath(docData).Extract("tags").ExtractIndexText(0).Eq("go")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("chained Extract = %v, want d1 and d2 (both start with \"go\")", ids(got))
	}
}

func TestSQLiteJSONKeyExists(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(KeyExists(docData, "meta")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("KeyExists(meta) = %d rows, want 3", len(got))
	}
}

func TestSQLiteJSONKeyExistsMissing(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(KeyExists(docData, "nope")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("KeyExists(nope) = %d rows, want 0", len(got))
	}
}

func TestSQLiteJSONArrayContains(t *testing.T) {
	ctx, conn := newDocsDB(t)

	// ArrayContains over the WHOLE document iterates the document's
	// top-level values (json_each on an object yields its entries); "go" is
	// nested under "tags", so the whole-document form matches nothing.
	got, err := orm.From(docs).Where(ArrayContains(docData, "go")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ArrayContains(go) over whole object = %d rows, want 0", len(got))
	}

	// ArrayContains over an array-typed column matches array elements.
	_, err = conn.Exec(ctx, `INSERT INTO docs (id, data) VALUES (?, ?)`, "d4", `["go","rust"]`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err = orm.From(docs).Where(ArrayContains(docData, "go")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d4" {
		t.Fatalf("ArrayContains(go) over array = %v, want exactly d4", ids(got))
	}
}

func TestSQLiteJSONArrayContainsWithPath(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(JSONPath(docData).Extract("tags").ArrayContains("json")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("ArrayContains(tags, json) = %v, want exactly d1", ids(got))
	}
}

func TestSQLiteJSONType(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(Type(docData).Eq("object")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Type(data)=object = %d rows, want 3", len(got))
	}

	got, err = orm.From(docs).Where(JSONPath(docData).Extract("tags").Type().Eq("array")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Type(tags)=array = %d rows, want 3", len(got))
	}
}

func TestSQLiteJSONIsNull(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(ExtractText(docData, "missing").IsNull()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("ExtractText(missing).IsNull() = %d rows, want 3", len(got))
	}

	got, err = orm.From(docs).Where(ExtractText(docData, "missing").IsNotNull()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ExtractText(missing).IsNotNull() = %d rows, want 0", len(got))
	}
}

func TestSQLiteJSONIn(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(ExtractText(docData, "role").In("admin", "user")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("ExtractText(role).In(admin,user) = %d rows, want 3", len(got))
	}

	got, err = orm.From(docs).Where(ExtractText(docData, "role").In("admin")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("ExtractText(role).In(admin) = %v, want exactly d1", ids(got))
	}
}

func TestSQLiteJSONComposesWithAnd(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).
		Where(orm.And(
			ExtractText(docData, "role").Eq("user"),
			JSONPath(docData).Extract("tags").ArrayContains("go"),
		)).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d2" {
		t.Fatalf("And(role=user, ArrayContains(go)) = %v, want exactly d2", ids(got))
	}
}

func TestSQLiteJSONCount(t *testing.T) {
	ctx, conn := newDocsDB(t)

	n, err := orm.From(docs).Where(ExtractText(docData, "role").Eq("user")).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count(role=user) = %d, want 2", n)
	}
}

func ids(docs []*doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}

	return out
}

// TestSQLiteBuilderCoverage exercises every builder and comparison so the
// full fluent surface renders and runs against real SQLite.
func TestSQLiteBuilderCoverage(t *testing.T) {
	ctx, conn := newDocsDB(t)

	run := func(name string, pred orm.Predicate[doc]) {
		t.Helper()

		if _, err := orm.From(docs).Where(pred).All(ctx, conn); err != nil {
			t.Fatalf("%s: All: %v", name, err)
		}
	}

	p := JSONPath(docData)

	run("Extract", Extract(docData, "name").Eq(`"alice"`))
	run("ExtractIndex", ExtractIndex(docData, 0).Eq(`"x"`))
	run("PathExtract", PathExtract(docData, "meta", "active").Eq(`true`))
	run("ExtractText", ExtractText(docData, "name").Eq("alice"))
	run("ExtractIndexText", ExtractIndexText(docData, 0).Eq("go"))
	run("PathExtractText", PathExtractText(docData, "meta").Neq("x"))
	run("KeyExists", KeyExists(docData, "name"))
	run("ArrayContains", ArrayContains(docData, "go"))
	run("Type", Type(docData).Eq("object"))

	run("Path.Extract", p.Extract("name").Eq(`"alice"`))
	run("Path.ExtractIndex", p.ExtractIndex(0).Eq(`"x"`))
	run("Path.PathExtract", p.PathExtract("meta").Eq(`{}`))
	run("Path.ExtractText", p.ExtractText("name").Eq("alice"))
	run("Path.ExtractIndexText", p.ExtractIndexText(0).Eq("go"))
	run("Path.PathExtractText", p.PathExtractText("meta").Neq("x"))
	run("Path.KeyExists", p.KeyExists("name"))
	run("Path.ArrayContains", p.ArrayContains("go"))
	run("Path.Eq", p.Eq(`{"a":1}`))
	run("Path.Neq", p.Neq(`{"a":1}`))
	run("Path.IsNull", p.IsNull())
	run("Path.IsNotNull", p.IsNotNull())

	tx := p.ExtractText("name")
	run("Text.Eq", tx.Eq("alice"))
	run("Text.Neq", tx.Neq("bob"))
	run("Text.Gt", tx.Gt("a"))
	run("Text.Gte", tx.Gte("alice"))
	run("Text.Lt", tx.Lt("z"))
	run("Text.Lte", tx.Lte("alice"))
	run("Text.In", tx.In("alice", "bob"))
	run("Text.IsNull", tx.IsNull())
	run("Text.IsNotNull", tx.IsNotNull())
}
