package sqlite

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	orm "github.com/zenta-dev/zever/orm"
)

// doc is a fixture entity over an FTS5 virtual table, mirroring the shape
// schema codegen generates for a `body: string` field. The
// FTS5 table is created via raw DDL (the virtual-table shape is not
// expressible in the schema DSL); orm queries over it render an
// ordinary SELECT.
type doc struct {
	ID    string
	Title string
	Body  string
}

func (d *doc) Scan(row orm.Row) error {
	return row.Scan(&d.ID, &d.Title, &d.Body)
}

var (
	docs     = orm.NewTable[doc]("docs", []string{"id", "title", "body"})
	docTitle = orm.NewColumn[doc, string]("docs", "title")
	docBody  = orm.NewColumn[doc, string]("docs", "body")
)

// newDocsDB opens an in-memory sqlite database with an FTS5 virtual table
// seeded with four documents, proving the MATCH round-trip end to end.
func newDocsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err := conn.Exec(ctx, `CREATE VIRTUAL TABLE docs USING fts5(id, title, body)`); err != nil {
		t.Fatalf("create fts5 table: %v", err)
	}

	rows := []struct{ id, title, body string }{
		{"d1", "Go release", "Go 1.27 ships generators and iterators"},
		{"d2", "Rust borrow checker", "memory safety without a garbage collector"},
		{"d3", "Distributed systems", "consensus, replication and the cap theorem"},
		{"d4", "Go on the web", "writing go http services with the go standard library"},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO docs (id, title, body) VALUES (?, ?, ?)`, r.id, r.title, r.body); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}

func TestSQLiteFTS5Match(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(Match(docBody, "go")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := map[string]bool{"d1": true, "d4": true}
	if len(got) != len(want) {
		t.Fatalf("Match(body, go) = %v, want exactly d1 and d4", ids(got))
	}

	for _, d := range got {
		if !want[d.ID] {
			t.Fatalf("Match(body, go) returned unexpected %q", d.ID)
		}
	}
}

func TestSQLiteFTS5MatchColumn(t *testing.T) {
	ctx, conn := newDocsDB(t)

	// Column-scoped MATCH: only docs whose TITLE matches, not the body.
	got, err := orm.From(docs).Where(Match(docTitle, "Go")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := map[string]bool{"d1": true, "d4": true}
	if len(got) != len(want) {
		t.Fatalf("Match(title, Go) = %v, want exactly d1 and d4", ids(got))
	}

	for _, d := range got {
		if !want[d.ID] {
			t.Fatalf("Match(title, Go) returned unexpected %q", d.ID)
		}
	}
}

func TestSQLiteFTS5MatchPhrase(t *testing.T) {
	ctx, conn := newDocsDB(t)

	// FTS5's native boolean/phrase syntax flows through the bound arg.
	got, err := orm.From(docs).Where(Match(docBody, `"memory safety"`)).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "d2" {
		t.Fatalf("Match(body, \"memory safety\") = %v, want exactly d2", ids(got))
	}
}

func TestSQLiteFTS5MatchBoolean(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).Where(Match(docBody, "go AND http")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// FTS5's AND maps to intersection: d4 is the only body with both "go"
	// and "http".
	if len(got) != 1 || got[0].ID != "d4" {
		t.Fatalf("Match(body, go AND http) = %v, want exactly d4", ids(got))
	}
}

func TestSQLiteFTS5RankOrdering(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody)).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// Both docs match; ORDER BY rank DESC runs FTS5's bm25 relevance
	// scoring. The exact scores are implementation-defined, but bm25
	// length-normalizes, so d1's single "go" in a short body outranks d4's
	// two "go"s in a longer one -- a deterministic ordering, which is what
	// the test asserts.
	if len(got) != 2 {
		t.Fatalf("ranked Match(body, go) = %v, want 2 rows", ids(got))
	}

	if got[0].ID != "d1" || got[1].ID != "d4" {
		t.Fatalf("ranked order = [%s %s], want [d1 d4]", got[0].ID, got[1].ID)
	}
}

func TestSQLiteFTS5MatchCount(t *testing.T) {
	ctx, conn := newDocsDB(t)

	n, err := orm.From(docs).Where(Match(docBody, "go")).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count(Match(body, go)) = %d, want 2", n)
	}
}

func TestSQLiteFTS5MatchComposesWithAnd(t *testing.T) {
	ctx, conn := newDocsDB(t)

	got, err := orm.From(docs).
		Where(orm.And(Match(docBody, "go"), orm.StartsWith(docTitle, "Go"))).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("And(Match(body, go), title LIKE 'Go%%') = %v, want d1 and d4", ids(got))
	}
}

func ids(docs []*doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}

	return out
}
