package main

import (
	"context"
	"sort"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/json_query/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	sqlitejson "github.com/zenta-dev/zever/orm/json/sqlite"
)

func newTestDB(ctx context.Context, t *testing.T) db.DB {
	t.Helper()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return conn
}

func ids(docs []*gen.Doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}
	sort.Strings(out)
	return out
}

func assertIDs(t *testing.T, what string, got []*gen.Doc, want ...string) {
	t.Helper()

	sorted := append([]string(nil), want...)
	sort.Strings(sorted)

	have := ids(got)
	if len(have) != len(sorted) {
		t.Fatalf("%s = %v, want %v", what, have, sorted)
	}
	for i := range sorted {
		if have[i] != sorted[i] {
			t.Fatalf("%s = %v, want %v", what, have, sorted)
		}
	}
}

func TestFilterByNestedKey(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").ExtractText("name").Eq("Grace")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertIDs(t, "author Grace", got, "d2")
}

func TestFilterByKeyExistence(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(sqlitejson.KeyExists(gen.DocCols.Data, "draft")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertIDs(t, "draft key", got, "d1", "d2", "d3")
}

func TestFilterByArrayMembership(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("tags").ArrayContains("go")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertIDs(t, "tags go", got, "d1", "d3")
}

func TestCheckValueType(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").Type().Eq("object")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertIDs(t, "author object", got, "d1", "d2", "d3")
}

func TestComposeJSONPredicates(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(orm.And(
			sqlitejson.JSONPath(gen.DocCols.Data).Extract("tags").ArrayContains("go"),
			sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").ExtractText("name").Eq("Ada"),
		)).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertIDs(t, "go by Ada", got, "d1")
}
