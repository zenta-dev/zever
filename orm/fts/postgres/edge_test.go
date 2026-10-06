package postgres

import (
	"reflect"
	"strings"
	"testing"

	orm "github.com/zenta-dev/zever/orm"
)

func argsEqual(a, b []any) bool {
	return reflect.DeepEqual(a, b)
}

func TestPostgresFTSMatchEmptyText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Match(docBody, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{""}) {
		t.Fatalf("args = %#v, want [\"\"]", capture.lastArgs)
	}
}

func TestPostgresFTSMatchSpecialCharsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := `sql & "acid" | !go 'quoted' \ % _`
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if strings.Contains(capture.lastQuery, text) {
		t.Fatalf("query text inlined into SQL: %q", capture.lastQuery)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestPostgresFTSMatchTSQueryInvalidSyntaxStaysBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// Rendering never validates tsquery syntax; the database rejects it.
	// The predicate must bind the text verbatim regardless.
	text := "sql &"
	if _, err := orm.From(docs).Where(MatchTSQuery(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ to_tsquery('english', $1)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestPostgresFTSMatchUnicodeText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := "café ☕ 日本語"
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestPostgresFTSMatchComposesWithOr(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).
		Where(orm.Or(Match(docBody, "go"), Match(docBody, "rust"))).
		All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "body" FROM "docs" WHERE (to_tsvector('english', "body") @@ plainto_tsquery('english', $1) OR to_tsvector('english', "body") @@ plainto_tsquery('english', $2))`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{"go", "rust"}) {
		t.Fatalf("args = %#v, want [go rust]", capture.lastArgs)
	}
}

func TestPostgresFTSRankEmptyText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody, "")).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1) ORDER BY ts_rank(to_tsvector('english', "body"), plainto_tsquery('english', $2)) DESC`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{"go", ""}) {
		t.Fatalf("args = %#v, want [go \"\"]", capture.lastArgs)
	}
}

func TestPostgresFTSRankSpecialCharsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := `go & "acid" | rust`
	_, err := orm.From(docs).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody, text)).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if strings.Contains(capture.lastQuery, text) {
		t.Fatalf("rank text inlined into SQL: %q", capture.lastQuery)
	}

	if !argsEqual(capture.lastArgs, []any{"go", text}) {
		t.Fatalf("args = %#v, want [go %q]", capture.lastArgs, text)
	}
}

func TestPostgresFTSMatchLongText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := strings.Repeat("word ", 500)
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("long text not bound verbatim: got %d args", len(capture.lastArgs))
	}
}
