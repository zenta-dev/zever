package orm

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
)

func TestJSONStepConstructors(t *testing.T) {
	t.Parallel()

	k := JSONKey("name")
	if k.Key != "name" || k.IsIndex {
		t.Fatalf("JSONKey = %+v, want key step", k)
	}

	idx := JSONIndex(2)
	if idx.Index != 2 || !idx.IsIndex {
		t.Fatalf("JSONIndex = %+v, want index step", idx)
	}
}

func TestJSONIndexArrayQuery(t *testing.T) {
	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	_, err = conn.Exec(ctx, `CREATE TABLE docs (id text, data text)`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	seed := []struct{ id, data string }{
		{"d1", `{"tags":["go","json"]}`},
		{"d2", `{"tags":["python"]}`},
	}
	for _, r := range seed {
		_, err = conn.Exec(ctx, `INSERT INTO docs (id, data) VALUES (?, ?)`, r.id, r.data)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	docs := NewTable[arrDoc]("docs", []string{"id", "data"})

	got, err := From(docs).
		Where(NewJSONPredicate[arrDoc]("docs", "data",
			JSONExpr{Op: JSONExtractText, Steps: []JSONStep{JSONKey("tags"), JSONIndex(0)}},
			Eq, "go")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("array index query = %v, want exactly d1", got)
	}
}

type arrDoc struct {
	ID   string
	Data string
}

func (d *arrDoc) Scan(row Row) error { return row.Scan(&d.ID, &d.Data) }
