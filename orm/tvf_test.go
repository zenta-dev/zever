package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestTVFSourceCapabilityInterfacesCompile guards the two in-tree dialects
// against signature drift: both must implement BOTH table-valued JSON
// source capability interfaces, so a missing method fails to compile rather
// than failing at a type assertion.
func TestTVFSourceCapabilityInterfacesCompile(_ *testing.T) {
	var (
		_ dialect.JSONEachDialect         = sqlite.New()
		_ dialect.JSONEachDialect         = postgres.New()
		_ dialect.JSONSetReturningDialect = sqlite.New()
		_ dialect.JSONSetReturningDialect = postgres.New()
	)
}

// fakeTVFSource is a minimal TableValuedSource[fakeTVFRow] for exercising the
// shared builder without a JSON dialect package.
type fakeTVFSource struct {
	alias string
	cols  []string
	sql   string
	gate  error
}

func (s fakeTVFSource) SrcAlias() string     { return s.alias }
func (s fakeTVFSource) SrcColumns() []string { return s.cols }
func (s fakeTVFSource) NewRow() fakeTVFRow   { return fakeTVFRow{} }

func (s fakeTVFSource) RenderSource(dialect.Dialect) (string, error) {
	if s.gate != nil {
		return "", s.gate
	}

	return s.sql, nil
}

type fakeTVFRow struct {
	A string
	B string
}

func (r *fakeTVFRow) Scan(row Row) error { return row.Scan(&r.A, &r.B) }

// TestTVFJoinRender renders a CROSS TVF join and asserts the source function
// text, the qualified source columns, and a source-scoped bound predicate.
func TestTVFJoinRender(t *testing.T) {
	src := fakeTVFSource{alias: "x", cols: []string{"a", "b"}, sql: "fn(\"widgets\".\"bio\")"}

	j := JoinTVF(From(widgets), src).
		Where(widgetID.Eq("w1")).
		WhereSource(NewColumn[fakeTVFRow, string]("x", "a").Eq("v")).
		OrderBySource(NewColumn[fakeTVFRow, string]("x", "b").Desc()).
		Limit(3)

	q, args, err := j.render(sqlite.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	want := `SELECT "widgets"."id", "widgets"."name", "widgets"."quantity", "widgets"."bio", "x"."a", "x"."b" ` +
		`FROM "widgets" CROSS JOIN fn("widgets"."bio") AS "x" ` +
		`WHERE ("widgets"."id" = ? AND "x"."a" = ?) ORDER BY "x"."b" DESC LIMIT ?`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 3 || args[0] != "w1" || args[1] != "v" || args[2] != 3 {
		t.Fatalf("args = %#v, want [w1 v 3]", args)
	}
}

// TestTVFJoinUnsupportedDialect proves the source's own gate surfaces as a
// typed dialect.ErrUnsupportedByDialect through the public All path.
func TestTVFJoinUnsupportedDialect(t *testing.T) {
	ctx := context.Background()

	src := fakeTVFSource{
		alias: "x",
		cols:  []string{"a"},
		gate:  dialect.ErrUnsupportedByDialect,
	}

	_, err := JoinTVF(From(widgets), src).All(ctx, mockExec{dialectName: "sqlite"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("All err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestTVFJoinValidation rejects a source with no alias or no output columns
// with a construction-time typed error, never invalid SQL.
func TestTVFJoinValidation(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		src  fakeTVFSource
		want string
	}{
		{"empty alias", fakeTVFSource{cols: []string{"a"}, sql: "fn()"}, "non-empty alias"},
		{"no columns", fakeTVFSource{alias: "x", sql: "fn()"}, "no output columns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := JoinTVF(From(widgets), tc.src).All(ctx, mockExec{dialectName: "sqlite"})
			if err == nil {
				t.Fatalf("All err = nil, want an error mentioning %q", tc.want)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestTVFJoinChainModifiers proves TVFJoin2's full modifier chain composes
// and renders: doubly-set source filters, both ORDER BY lists, Limit and
// Offset all reach the statement.
func TestTVFJoinChainModifiers(t *testing.T) {
	src := fakeTVFSource{alias: "x", cols: []string{"a", "b"}, sql: "fn(\"widgets\".\"bio\")"}

	j := JoinTVF(From(widgets), src).
		Where(widgetID.Eq("w1")).
		WhereSource(NewColumn[fakeTVFRow, string]("x", "a").Eq("v")).
		WhereSource(NewColumn[fakeTVFRow, string]("x", "b").Eq("w")).
		OrderBy(widgetID.Asc()).
		OrderBySource(NewColumn[fakeTVFRow, string]("x", "b").Desc()).
		Limit(3).
		Offset(1)

	q, _, err := j.render(sqlite.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, frag := range []string{"CROSS JOIN", "ORDER BY", "LIMIT", "OFFSET"} {
		if !strings.Contains(q, frag) {
			t.Fatalf("query %q missing %q", q, frag)
		}
	}
}

// TestLeftJoinTVFSurface proves the LEFT TVF join end to end: the full
// modifier chain renders, All scans matched and unmatched source rows into
// Some/None, Stream drains, and Explain runs.
func TestLeftJoinTVFSurface(t *testing.T) {
	ctx := context.Background()

	src := fakeTVFSource{alias: "x", cols: []string{"a", "b"}, sql: "fn(\"widgets\".\"bio\")"}

	j := LeftJoinTVF(From(widgets), src).
		Where(widgetID.Eq("w1")).
		WhereSource(NewColumn[fakeTVFRow, string]("x", "a").Eq("v")).
		WhereSource(NewColumn[fakeTVFRow, string]("x", "b").Eq("w")).
		OrderBy(widgetID.Asc()).
		OrderBySource(NewColumn[fakeTVFRow, string]("x", "b").Desc()).
		Limit(3).
		Offset(0)

	q, _, err := j.render(sqlite.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(q, "LEFT JOIN") {
		t.Fatalf("query %q missing LEFT JOIN", q)
	}

	matched := &ormStubDB{
		mockExec: mockExec{dialectName: "sqlite"},
		rows: &stubRows{values: [][]any{
			{"w1", "Alpha", int64(10), "first", "v", "w"},
		}},
	}

	rows, err := LeftJoinTVF(From(widgets), src).All(ctx, matched)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 1 || rows[0].A.ID != "w1" || !rows[0].B.IsSome() {
		t.Fatalf("All = %+v, want 1 matched row w1/v", rows)
	}

	if b, ok := rows[0].B.Get(); !ok || b.A != "v" {
		t.Fatalf("matched source = (%+v, %v), want (v, true)", b, ok)
	}

	unmatched := &ormStubDB{
		mockExec: mockExec{dialectName: "sqlite"},
		rows: &stubRows{values: [][]any{
			{"w1", "Alpha", int64(10), "first", nil, nil},
		}},
	}

	rows, err = LeftJoinTVF(From(widgets), src).All(ctx, unmatched)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 1 || rows[0].B.IsSome() {
		t.Fatalf("All = %+v, want 1 unmatched row with None source", rows)
	}

	bad := &ormStubDB{
		mockExec: mockExec{dialectName: "sqlite"},
		rows:     &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first", nil, "w"}}, scanErr: nil},
	}

	if _, err := LeftJoinTVF(From(widgets), src).All(ctx, bad); err == nil {
		t.Fatal("All with a half-null source row succeeded, want a scan error")
	}

	if err := drainJoinStream(LeftJoinTVF(From(widgets), src).Stream(ctx, mockExec{dialectName: "sqlite"})); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if err := drainJoinStream(LeftJoinTVF(From(widgets), src).Stream(ctx, fakeDB{})); err == nil {
		t.Fatal("Stream on an unresolvable dialect succeeded, want an error")
	}

	if _, err := LeftJoinTVF(From(widgets), src).Explain(ctx, mockExec{dialectName: "sqlite"}); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if _, err := LeftJoinTVF(From(widgets), src).ExplainAnalyze(ctx, mockExec{dialectName: "postgres"}); err != nil {
		t.Fatalf("ExplainAnalyze: %v", err)
	}

	if _, err := JoinTVF(From(widgets), src).Explain(ctx, mockExec{dialectName: "sqlite"}); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	if _, err := JoinTVF(From(widgets), src).ExplainAnalyze(ctx, fakeDB{}); err == nil {
		t.Fatal("ExplainAnalyze on an unresolvable dialect succeeded, want an error")
	}

	if err := drainJoinStream(JoinTVF(From(widgets), src).Stream(ctx, mockExec{dialectName: "sqlite"})); err != nil {
		t.Fatalf("Stream: %v", err)
	}
}

// TestTVFJoinScanErrorPaths drives rows, entity and source scan failures
// through both TVF join scans.
func TestTVFJoinScanErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	src := fakeTVFSource{alias: "x", cols: []string{"a", "b"}, sql: "fn(\"widgets\".\"bio\")"}
	e := mockExec{dialectName: "sqlite"}

	okStub := &ormStubDB{mockExec: e, rows: &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first", "v", "w"}}}}

	rows, err := JoinTVF(From(widgets), src).All(ctx, okStub)
	if err != nil {
		t.Fatalf("JoinTVF All: %v", err)
	}

	if len(rows) != 1 || rows[0].A.ID != "w1" || rows[0].B.A != "v" || rows[0].B.B != "w" {
		t.Fatalf("JoinTVF All = %+v, want 1 row w1/(v,w)", rows)
	}

	newScanErr := func() *ormStubDB {
		return &ormStubDB{mockExec: e, rows: &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first", "v", "w"}}, scanErr: boom}}
	}

	if _, err := JoinTVF(From(widgets), src).All(ctx, newScanErr()); !errors.Is(err, boom) {
		t.Fatalf("JoinTVF All err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := LeftJoinTVF(From(widgets), src).All(ctx, newScanErr()); !errors.Is(err, boom) {
		t.Fatalf("LeftJoinTVF All err = %v, want errors.Is(err, boom)", err)
	}

	newBadA := func() *ormStubDB {
		return &ormStubDB{mockExec: e, rows: &stubRows{values: [][]any{{nil, "Alpha", int64(10), "first", "v", "w"}}}}
	}

	if _, err := JoinTVF(From(widgets), src).All(ctx, newBadA()); err == nil {
		t.Fatal("JoinTVF All with a null entity id succeeded, want a scan error")
	}

	if _, err := LeftJoinTVF(From(widgets), src).All(ctx, newBadA()); err == nil {
		t.Fatal("LeftJoinTVF All with a null entity id succeeded, want a scan error")
	}

	newBadB := func() *ormStubDB {
		return &ormStubDB{mockExec: e, rows: &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first", nil, "w"}}}}
	}

	if _, err := JoinTVF(From(widgets), src).All(ctx, newBadB()); err == nil {
		t.Fatal("JoinTVF All with a null source column succeeded, want a scan error")
	}

	if _, err := LeftJoinTVF(From(widgets), src).All(ctx, newBadB()); err == nil {
		t.Fatal("LeftJoinTVF All with a half-null source row succeeded, want a scan error")
	}
}
