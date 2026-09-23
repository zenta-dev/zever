// Command app is the JSON-query example: a small documents table whose
// `data` column stores JSON text (schema/json_query.zen, codegen'd by the
// zenorm backend into generated/zenorm/orm/gen/app), queried with the typed
// sqlite json1 helpers from github.com/zenta-dev/zever/orm/json/sqlite.
//
// It seeds a few JSON documents, then exercises the json1 surface end to
// end against a real (in-memory) SQLite: filtering by a nested key,
// key existence, array membership via json_each, value typing via
// json_type, and a composed AND of two JSON predicates — all as ordinary
// orm.From(...).Where(...) predicates, never as hand-written SQL strings.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/json_query/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	sqlitejson "github.com/zenta-dev/zever/orm/json/sqlite"
)

func main() {
	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := seed(ctx, conn); err != nil {
		die(err)
	}

	if err := filterByNestedKey(ctx, conn); err != nil {
		die(err)
	}

	if err := filterByKeyExistence(ctx, conn); err != nil {
		die(err)
	}

	if err := filterByArrayMembership(ctx, conn); err != nil {
		die(err)
	}

	if err := checkValueType(ctx, conn); err != nil {
		die(err)
	}

	if err := composeJSONPredicates(ctx, conn); err != nil {
		die(err)
	}
}

// seed creates the docs table and inserts three JSON documents. The JSON
// text is inserted with orm.Insert/orm.Set — the same typed surface as
// everything else, with the document carried as a plain string value.
func seed(ctx context.Context, conn db.DB) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE docs (id text, title text, data text)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	docs := []struct {
		id, title, data string
	}{
		{
			"d1", "Go at the JSON Zoo",
			`{"author":{"name":"Ada","email":"ada@example.com"},"tags":["go","json"],"views":42,"draft":true}`,
		},
		{
			"d2", "SQLite JSON1 Field Guide",
			`{"author":{"name":"Grace","email":"grace@example.com"},"tags":["sqlite","json","sql"],"views":150,"draft":true}`,
		},
		{
			"d3", "Typed ORM Predicates",
			`{"author":{"name":"Margaret","email":"margaret@example.com"},"tags":["go","orm"],"views":7,"draft":false}`,
		},
	}

	for _, d := range docs {
		insert := orm.InsertInto(gen.Docs).Values(
			orm.Set(gen.DocCols.ID, d.id),
			orm.Set(gen.DocCols.Title, d.title),
			orm.Set(gen.DocCols.Data, d.data),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert %s: %w", d.id, err)
		}
	}

	return nil
}

// filterByNestedKey proves json_extract-based filtering: find every doc
// whose nested author.name equals a value.
func filterByNestedKey(ctx context.Context, conn db.DB) error {
	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").ExtractText("name").Eq("Grace")).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("filter by nested key: %w", err)
	}

	fmt.Printf("docs authored by Grace:\n")

	for _, d := range got {
		fmt.Printf("  %s (%s)\n", d.ID, d.Title)
	}

	return nil
}

// filterByKeyExistence proves key-existence filtering: every doc whose
// document has a top-level "draft" key. On sqlite this renders as
// json_extract(data, '$.draft') IS NOT NULL.
func filterByKeyExistence(ctx context.Context, conn db.DB) error {
	got, err := orm.From(gen.Docs).
		Where(sqlitejson.KeyExists(gen.DocCols.Data, "draft")).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("filter by key existence: %w", err)
	}

	fmt.Printf("docs with a top-level \"draft\" key (%d):", len(got))

	for _, d := range got {
		fmt.Printf(" %s", d.ID)
	}

	fmt.Println()

	return nil
}

// filterByArrayMembership proves json_each-based filtering: every doc whose
// tags array contains "go". Rendered as
// EXISTS (SELECT 1 FROM json_each(data, '$.tags') WHERE value = ?).
func filterByArrayMembership(ctx context.Context, conn db.DB) error {
	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("tags").ArrayContains("go")).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("filter by array membership: %w", err)
	}

	fmt.Printf("docs tagged \"go\" (%d):", len(got))

	for _, d := range got {
		fmt.Printf(" %s", d.ID)
	}

	fmt.Println()

	return nil
}

// checkValueType proves json_type-based filtering: every doc whose
// author value is a JSON object.
func checkValueType(ctx context.Context, conn db.DB) error {
	got, err := orm.From(gen.Docs).
		Where(sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").Type().Eq("object")).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("filter by value type: %w", err)
	}

	fmt.Printf("docs whose author is a JSON object (%d):", len(got))

	for _, d := range got {
		fmt.Printf(" %s", d.ID)
	}

	fmt.Println()

	return nil
}

// composeJSONPredicates proves JSON predicates compose with orm.And like
// any other predicate: find every doc whose tags contain "go" AND whose
// author.name is "Ada".
func composeJSONPredicates(ctx context.Context, conn db.DB) error {
	got, err := orm.From(gen.Docs).
		Where(orm.And(
			sqlitejson.JSONPath(gen.DocCols.Data).Extract("tags").ArrayContains("go"),
			sqlitejson.JSONPath(gen.DocCols.Data).Extract("author").ExtractText("name").Eq("Ada"),
		)).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("compose JSON predicates: %w", err)
	}

	fmt.Printf("docs tagged \"go\" by Ada (%d):", len(got))

	for _, d := range got {
		fmt.Printf(" %s", d.ID)
	}

	fmt.Println()

	return nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
