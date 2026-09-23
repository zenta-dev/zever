package orm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

func TestQueryAll(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("All returned %d rows, want 3", len(got))
	}

	if got[0].ID != "w1" || got[1].ID != "w2" || got[2].ID != "w3" {
		t.Fatalf("All order = %v, want w1,w2,w3", []string{got[0].ID, got[1].ID, got[2].ID})
	}

	bio1, ok1 := got[0].Bio.Get()
	if !ok1 || bio1 != "first" {
		t.Fatalf("got[0].Bio = (%q, %v), want (\"first\", true)", bio1, ok1)
	}

	if got[1].Bio.IsSome() {
		t.Fatalf("got[1].Bio.IsSome() = true, want false (NULL bio)")
	}
}

// TestQueryAllWithLimitReturnsExactlyLimitedRows guards the capacity-hint
// optimization in All(): pre-sizing the result slice from q.limit must not
// change the returned row count/content, whether the limit is below,
// equal to, or above the actual number of matching rows.
func TestQueryAllWithLimitReturnsExactlyLimitedRows(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).OrderBy(widgetID.Asc()).Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("All with Limit(2) returned %d rows, want 2", len(got))
	}

	if got[0].ID != "w1" || got[1].ID != "w2" {
		t.Fatalf("All with Limit(2) order = %v, want w1,w2", []string{got[0].ID, got[1].ID})
	}

	// Limit greater than the actual row count must not over-allocate into
	// a wrong length -- append still grows correctly past a pre-sized cap.
	got, err = From(widgets).OrderBy(widgetID.Asc()).Limit(1000).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("All with Limit(1000) over 3 actual rows returned %d, want 3", len(got))
	}
}

func TestQueryWhere(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).Where(widgetName.Eq("Beta")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("Where(name=Beta) = %v, want exactly w2", got)
	}
}

func TestQueryWhereChaining(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).
		Where(widgetQty.Gte(20)).
		Where(widgetQty.Lte(20)).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("chained Where = %v, want exactly w2", got)
	}
}

func TestQueryWhereNullableColumnIsNull(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).Where(widgetBio.IsNull()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("Where(bio IS NULL) = %v, want exactly w2", got)
	}
}

func TestQueryLimitOffset(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).OrderBy(widgetID.Asc()).Limit(1).Offset(1).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" {
		t.Fatalf("Limit(1).Offset(1) = %v, want exactly w2", got)
	}
}

// TestQueryLimitZeroIsUnlimited pins the zero-value-is-unset convention:
// Limit(0) (and Offset(0)) render NO clause, so the query returns every
// row -- Limit(0) is "no limit", NOT "zero rows".
func TestQueryLimitZeroIsUnlimited(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := From(widgets).OrderBy(widgetID.Asc()).Limit(0).Offset(0).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("Limit(0).Offset(0) = %d rows, want all 3 (zero-value = unset = unlimited)", len(got))
	}
}

func TestQueryFirst(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	row, ok, err := From(widgets).Where(widgetID.Eq("w3")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if !ok || row.ID != "w3" {
		t.Fatalf("First = (%v, %v), want w3", row, ok)
	}

	_, ok2, err2 := From(widgets).Where(widgetID.Eq("nope")).First(ctx, conn)
	if err2 != nil {
		t.Fatalf("First: %v", err2)
	}

	if ok2 {
		t.Fatalf("First on no match: ok = true, want false")
	}
}

func TestQueryFirstOrErr(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	row, err := From(widgets).Where(widgetID.Eq("w3")).FirstOrErr(ctx, conn)
	if err != nil {
		t.Fatalf("FirstOrErr: %v", err)
	}

	if row.ID != "w3" {
		t.Fatalf("FirstOrErr = %v, want w3", row)
	}

	_, err2 := From(widgets).Where(widgetID.Eq("nope")).FirstOrErr(ctx, conn)
	if err2 == nil {
		t.Fatalf("FirstOrErr on no match: err = nil, want an errors.Is(db.ErrNotFound) error")
	}

	if !errors.Is(err2, db.ErrNotFound) {
		t.Fatalf("FirstOrErr no-match error = %v, want errors.Is(err, db.ErrNotFound)", err2)
	}
}

func TestQueryCount(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	n, err := From(widgets).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 3 {
		t.Fatalf("Count = %d, want 3", n)
	}

	n2, err2 := From(widgets).Where(widgetQty.Gt(15)).Count(ctx, conn)
	if err2 != nil {
		t.Fatalf("Count: %v", err2)
	}

	if n2 != 2 {
		t.Fatalf("Count(qty>15) = %d, want 2", n2)
	}
}

func TestQueryExists(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	ok, err := From(widgets).Where(widgetID.Eq("w1")).Exists(ctx, conn)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !ok {
		t.Fatalf("Exists(w1) = false, want true")
	}

	ok2, err2 := From(widgets).Where(widgetID.Eq("nope")).Exists(ctx, conn)
	if err2 != nil {
		t.Fatalf("Exists: %v", err2)
	}

	if ok2 {
		t.Fatalf("Exists(nope) = true, want false")
	}
}

// TestQueryOrderByDoesNotShareBackingArray is the branching-safety proof:
// two Query values branched from one base via OrderBy must never share (or
// corrupt) each other's order slice.
func TestQueryOrderByDoesNotShareBackingArray(t *testing.T) {
	base := From(widgets).OrderBy(widgetID.Asc())

	branchA := base.OrderBy(widgetName.Asc())
	branchB := base.OrderBy(widgetQty.Desc())

	if len(base.order) != 1 {
		t.Fatalf("base.order mutated by branching: %v", base.order)
	}

	if len(branchA.order) != 2 || branchA.order[1].Column.Name() != "name" {
		t.Fatalf("branchA.order = %v, want [id, name]", branchA.order)
	}

	if len(branchB.order) != 2 || branchB.order[1].Column.Name() != "quantity" {
		t.Fatalf("branchB.order = %v, want [id, quantity]", branchB.order)
	}

	// The critical assertion: appending to branchA must not have touched
	// branchB's backing array (or vice versa) via shared capacity.
	if branchA.order[1].Column.Name() == branchB.order[1].Column.Name() {
		t.Fatalf("branchA and branchB unexpectedly share an order entry")
	}
}

// TestQueryWhereDoesNotMutateBase proves Where's copy-on-write rule: a base
// Query with no filter set stays filterless after a branch calls Where.
func TestQueryWhereDoesNotMutateBase(t *testing.T) {
	base := From(widgets)

	branch := base.Where(widgetID.Eq("w1"))

	if base.where.IsSet() {
		t.Fatalf("base.where.IsSet() = true after branching, want false (unmodified)")
	}

	if !branch.where.IsSet() {
		t.Fatalf("branch.where.IsSet() = false, want true")
	}
}

// TestQueryWhereCombinesWithAnd proves calling Where twice ANDs the two
// predicates rather than overwriting the first.
func TestQueryWhereCombinesWithAnd(t *testing.T) {
	q := From(widgets).Where(widgetQty.Gte(10)).Where(widgetQty.Lte(20))

	n := q.where.Render()
	if n.Kind != NCompound || n.Compound != CAnd || len(n.Children) != 2 {
		t.Fatalf("q.where.Render() = %+v, want a 2-child AND", n)
	}
}

func TestQueryUnsupportedDialect(t *testing.T) {
	ctx, _ := newWidgetsDB(t)

	_, err := From(widgets).All(ctx, fakeDB{})
	if err == nil {
		t.Fatalf("All with an unsupported dialect name succeeded, want an error")
	}
}

// TestQueryTablesampleUnsupportedOnSQLite proves TABLESAMPLE is a typed
// capability error on SQLite through the public All API.
func TestQueryTablesampleUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := From(widgets).Tablesample("SYSTEM", 10).All(ctx, conn)
	if err == nil {
		t.Fatalf("Tablesample on sqlite succeeded, want an error")
	}
}

// TestQueryTablesamplePostgresRenders proves a valid TABLESAMPLE renders
// through the Postgres path against a recording exec.
func TestQueryTablesamplePostgresRenders(t *testing.T) {
	ctx := context.Background()

	rec := &recordingExec{dialectName: "postgres"}

	_, _ = From(widgets).Tablesample("SYSTEM", 10).All(ctx, rec)

	q, _ := rec.last()

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" TABLESAMPLE SYSTEM (10)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestQueryForUpdateOfUnknownTableDropped proves a column without a home
// table contributes nothing to the FOR UPDATE OF list.
func TestQueryForUpdateOfUnknownTableDropped(t *testing.T) {
	ctx := context.Background()

	//lint:allow-unsafesql test: ident is from the test's own allowlist
	col, err := UnsafeIdent[widget, string]("name", []string{"id", "name", "quantity", "bio"})
	if err != nil {
		t.Fatalf("UnsafeIdent: %v", err)
	}

	rec := &recordingExec{dialectName: "postgres"}

	_, _ = From(widgets).ForUpdateOf(widgetID.Col(), col.Col()).All(ctx, rec)

	q, _ := rec.last()

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" FOR UPDATE OF "widgets"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestEncodeArgsSQLiteTime proves the sqlite time-encoding contract: a
// bound time.Time is converted to RFC3339Nano UTC text before reaching the
// driver, while every other value passes through untouched -- and that the
// Postgres path binds time.Time natively.
func TestEncodeArgsSQLiteTime(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	at := time.Date(2026, 6, 1, 12, 30, 45, 123456789, time.UTC)

	if _, err := conn.Exec(ctx, `CREATE TABLE events (id text, seen_at text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if _, err := conn.Exec(ctx, `INSERT INTO events (id, seen_at) VALUES (?, ?)`, "e1", at.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert: %v", err)
	}

	events := NewTable[eventRow]("events", []string{"id", "seen_at"})

	got, err := From(events).Where(eventSeenAt.Gte(at)).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].ID != "e1" {
		t.Fatalf("All = %v, want exactly e1", got)
	}

	if !got[0].SeenAt.Equal(at) {
		t.Fatalf("SeenAt = %v, want %v", got[0].SeenAt, at)
	}
}

// eventRow is a minimal entity with a timestamp column for the encodeArgs
// round-trip test.
type eventRow struct {
	ID     string
	SeenAt time.Time
}

func (e *eventRow) Scan(row Row) error {
	var raw string

	if err := row.Scan(&e.ID, &raw); err != nil {
		return err
	}

	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return err
	}

	e.SeenAt = t

	return nil
}

var eventSeenAt = NewColumn[eventRow, time.Time]("events", "seen_at")

// TestEncodeArgsPassthrough pins the two fast paths: no time.Time args
// returns the slice untouched, and a non-sqlite dialect never encodes.
func TestEncodeArgsPassthrough(t *testing.T) {
	d, err := resolveDialect(mockExec{dialectName: "sqlite"})
	if err != nil {
		t.Fatalf("resolveDialect: %v", err)
	}

	plain := []any{"a", int64(1)}
	if got := encodeArgs(d, plain); len(got) != 2 || got[0] != "a" {
		t.Fatalf("encodeArgs without times = %v, want passthrough", got)
	}

	pg, err := resolveDialect(mockExec{dialectName: "postgres"})
	if err != nil {
		t.Fatalf("resolveDialect: %v", err)
	}

	at := time.Date(2026, 6, 1, 12, 30, 45, 0, time.UTC)
	withTime := []any{at}
	if got := encodeArgs(pg, withTime); len(got) != 1 {
		t.Fatalf("encodeArgs postgres = %v, want passthrough", got)
	}

	if _, ok := withTime[0].(time.Time); !ok {
		t.Fatal("postgres encodeArgs must not rewrite the caller's slice")
	}

	encoded := encodeArgs(d, withTime)
	s, ok := encoded[0].(string)
	if !ok || s != at.Format(time.RFC3339) {
		t.Fatalf("encodeArgs sqlite = %#v, want RFC3339 text", encoded[0])
	}
}

// TestSetQueryLoggerCapturesSelectSQLAndArgs proves the debug hook captures
// the rendered SELECT and its bound args in execution order, and that the
// restore func disables logging again.
func TestSetQueryLoggerCapturesSelectSQLAndArgs(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var got []loggedQuery
	restore := collectLogs(&got)

	row, ok, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("First: row=%v ok=%v err=%v", row, ok, err)
	}

	if len(got) != 1 {
		t.Fatalf("logged %d queries, want 1", len(got))
	}

	q := got[0]
	if len(q.query) < 6 || q.query[:6] != "SELECT" {
		t.Fatalf("logged query %q missing SELECT", q.query)
	}

	if len(q.args) != 2 || q.args[0] != "w1" || q.args[1] != 1 {
		t.Fatalf("logged args = %v, want [w1 1] (WHERE value + implicit LIMIT 1)", q.args)
	}

	restore()

	if _, _, err := From(widgets).First(ctx, conn); err != nil {
		t.Fatalf("First after restore: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("logged %d queries after restore, want still 1 (logging disabled)", len(got))
	}
}

// TestSetQueryLoggerNilDisables proves installing a nil logger disables
// logging without failing.
func TestSetQueryLoggerNilDisables(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var got []loggedQuery
	restore := collectLogs(&got)
	defer restore()

	SetQueryLogger(nil)

	if _, err := From(widgets).Count(ctx, conn); err != nil {
		t.Fatalf("Count: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("logged %d queries with nil logger, want 0", len(got))
	}
}

// noWaitDialect is a LockingDialect with FOR UPDATE/SHARE but neither
// NOWAIT nor SKIP LOCKED, proving those suffixes gate independently of
// the lock strength.
type noWaitDialect struct{ mockDialect }

func (noWaitDialect) SupportsForUpdate() bool  { return true }
func (noWaitDialect) SupportsForShare() bool   { return true }
func (noWaitDialect) SupportsNoWait() bool     { return false }
func (noWaitDialect) SupportsSkipLocked() bool { return false }

func init() {
	if err := dialect.Register("mock-no-nowait", func() dialect.Dialect { return noWaitDialect{} }); err != nil {
		panic(err)
	}
}

// TestExtendedLockModesGate drives the Postgres-only lock strengths
// through sqlite and a base-only dialect (typed error) and through
// postgres (gate passes, empty result set yields no rows).
func TestExtendedLockModesGate(t *testing.T) {
	ctx := context.Background()

	forms := []struct {
		name  string
		apply func(Query[widget, *widget]) Query[widget, *widget]
	}{
		{"ForNoKeyUpdate", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForNoKeyUpdate() }},
		{"ForKeyShare", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForKeyShare() }},
		{"ForUpdateOf", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForUpdateOf(widgetID.Col()) }},
	}

	for _, f := range forms {
		for _, d := range []string{"sqlite", "mock-nocap", "mock-no-nowait"} {
			t.Run(f.name+"/"+d, func(t *testing.T) {
				_, err := f.apply(From(widgets)).All(ctx, mockExec{dialectName: d})
				if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
					t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
				}
			})
		}

		t.Run(f.name+"/postgres", func(t *testing.T) {
			if _, err := f.apply(From(widgets)).All(ctx, mockExec{dialectName: "postgres"}); err != nil {
				t.Fatalf("err = %v, want nil (postgres supports %s)", err, f.name)
			}
		})
	}
}

// TestExtendedLockModesRender pins the exact SQL each extended strength
// emits on postgres.
func TestExtendedLockModesRender(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		q    Query[widget, *widget]
		want string
	}{
		{"ForNoKeyUpdate", From(widgets).ForNoKeyUpdate(), `FOR NO KEY UPDATE`},
		{"ForKeyShare", From(widgets).ForKeyShare(), `FOR KEY SHARE`},
		{"ForUpdateOf", From(widgets).ForUpdateOf(widgetID.Col(), widgetName.Col()), `FOR UPDATE OF "widgets"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingExec{dialectName: "postgres"}

			if _, err := tc.q.All(ctx, rec); err != nil {
				t.Fatalf("All: %v", err)
			}

			q, _ := rec.last()
			if !strings.Contains(q, tc.want) {
				t.Fatalf("query %q missing %q", q, tc.want)
			}
		})
	}
}

// TestExtendedLockCountRejected proves Count/Exists reject the extended
// lock modes with ErrLockingNotSelect rather than silently dropping them.
func TestExtendedLockCountRejected(t *testing.T) {
	ctx := context.Background()

	for _, q := range []Query[widget, *widget]{
		From(widgets).ForNoKeyUpdate(),
		From(widgets).ForKeyShare(),
		From(widgets).ForUpdateOf(widgetID.Col()),
	} {
		if _, err := q.Count(ctx, mockExec{dialectName: "postgres"}); !errors.Is(err, ErrLockingNotSelect) {
			t.Fatalf("Count err = %v, want errors.Is(err, ErrLockingNotSelect)", err)
		}
	}
}

// TestExtendedLockSupportedDefaultUnreachable pins the defensive default:
// extendedLockSupported only ever receives the two extended modes from
// validateLockMode, so any other mode reports false.
func TestExtendedLockSupportedDefaultUnreachable(t *testing.T) {
	d, err := resolveDialect(mockExec{dialectName: "postgres"})
	if err != nil {
		t.Fatalf("resolveDialect: %v", err)
	}

	if extendedLockSupported(d, LockForUpdate) {
		t.Fatal("extendedLockSupported(postgres, LockForUpdate) = true, want false (only extended modes reach here)")
	}

	if !extendedLockSupported(d, LockForNoKeyUpdate) || !extendedLockSupported(d, LockForKeyShare) {
		t.Fatal("extendedLockSupported(postgres, extended) = false, want true")
	}
}

// TestLockingSuffixesGateIndependently proves NOWAIT/SKIP LOCKED gate on
// their own capability, not the lock strength's: a dialect with FOR UPDATE
// but no NOWAIT accepts the lock and rejects the suffix.
func TestLockingSuffixesGateIndependently(t *testing.T) {
	ctx := context.Background()

	if _, err := From(widgets).ForUpdate().All(ctx, mockExec{dialectName: "mock-no-nowait"}); err != nil {
		t.Fatalf("ForUpdate err = %v, want nil", err)
	}

	for _, tc := range []struct {
		name string
		q    Query[widget, *widget]
	}{
		{"NoWait", From(widgets).ForUpdate().NoWait()},
		{"SkipLocked", From(widgets).ForShare().SkipLocked()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.q.All(ctx, mockExec{dialectName: "mock-no-nowait"})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestFirstErrorPaths proves First and FirstOrErr surface execution errors
// instead of masking them as "no rows".
func TestFirstErrorPaths(t *testing.T) {
	ctx := context.Background()

	if _, _, err := From(widgets).First(ctx, fakeDB{}); err == nil {
		t.Fatal("First with an unsupported dialect succeeded, want an error")
	}

	if _, err := From(widgets).FirstOrErr(ctx, fakeDB{}); err == nil {
		t.Fatal("FirstOrErr with an unsupported dialect succeeded, want an error")
	}
}

// TestCountErrorPaths covers every Count failure: unresolvable dialect, a
// render-time failure (tuple arity), a query failure, rows.Err and Close.
func TestCountErrorPaths(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	if _, err := From(widgets).Count(ctx, fakeDB{}); err == nil {
		t.Fatal("Count with an unsupported dialect succeeded, want an error")
	}

	oneColumn := From(widgetOrders).Columns(orderWidgetID.Col())
	if _, err := From(widgets).Where(NewTuple(widgetID.Col(), widgetQty.Col()).In(oneColumn)).Count(ctx, conn); err == nil {
		t.Fatal("Count with an arity mismatch succeeded, want an error")
	}

	if _, err := From(widgets).Count(ctx, errQueryDB{}); err == nil {
		t.Fatal("Count with a failing Query succeeded, want an error")
	}

	if _, err := From(widgets).Count(ctx, &stubDB{rows: &stubRows{iterErr: errors.New("iter boom")}}); err == nil {
		t.Fatal("Count with a failing rows.Err succeeded, want an error")
	}

	if _, err := From(widgets).Count(ctx, &stubDB{rows: &stubRows{closeErr: errors.New("close boom")}}); err == nil {
		t.Fatal("Count with a failing Close succeeded, want an error")
	}
}

// TestFTSOrderTermRenders proves an FTS ranking order term renders the
// Postgres ts_rank expression with its bound query text.
func TestFTSOrderTermRenders(t *testing.T) {
	ctx := context.Background()

	rec := &recordingExec{dialectName: "postgres"}

	term := NewFTSOrderTerm(widgetBio.Col(), FTSExpr{Op: FTSRank, Query: "hello", Columns: []string{"bio"}}, true)

	_, _ = From(widgets).OrderBy(term).All(ctx, rec)

	q, args := rec.last()

	want := `ORDER BY ts_rank(to_tsvector('english', "bio"), plainto_tsquery('english', $1)) DESC`
	if !strings.Contains(q, want) {
		t.Fatalf("query = %q, want it to contain %q", q, want)
	}

	if len(args) != 1 || args[0] != "hello" {
		t.Fatalf("args = %#v, want [hello]", args)
	}
}

// TestFTSNodeWithColumnsCopied proves an FTS node carrying Columns
// shape-converts without aliasing the caller's slice.
func TestFTSNodeWithColumnsCopied(t *testing.T) {
	ctx := context.Background()

	rec := &recordingExec{dialectName: "postgres"}

	cols := []string{"bio"}
	fts := NewFTSPredicate[widget]("widgets", "bio", FTSExpr{Op: FTSMatch, Query: "hi", Columns: cols})

	_, _ = From(widgets).Where(fts).All(ctx, rec)

	cols[0] = "mutated"

	q, args := rec.last()
	if !strings.Contains(q, `"bio"`) {
		t.Fatalf("query = %q, want the searched column", q)
	}

	if len(args) != 1 || args[0] != "hi" {
		t.Fatalf("args = %#v, want [hi]", args)
	}
}

// TestOrderByRendersTypedColumnTemplate proves the fluent ORDER BY path
// renders a FIXED SQL template against the dialect, quoting only the
// schema-derived column name -- the payloads a caller can bind are values,
// never identifiers, so the SQL structure cannot be altered through
// OrderBy.
func TestOrderByRendersTypedColumnTemplate(t *testing.T) {
	rec := &recordingExec{dialectName: "postgres"}
	ctx := context.Background()

	_, _ = From(widgets).OrderBy(widgetName.Asc()).Limit(10).All(ctx, rec)
	q, _ := rec.last()

	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" ORDER BY "name" ASC LIMIT $1`

	if q != want {
		t.Fatalf("OrderBy template mismatch:\n got: %s\nwant: %s", q, want)
	}
}

// TestOrderTermAndAssignmentAreAnyColumnTyped pins the identifier-injection
// fix: OrderTerm.Column and Assignment.Column are AnyColumn[T] values built
// only from schema-derived columns, never raw strings.
func TestOrderTermAndAssignmentAreAnyColumnTyped(t *testing.T) {
	var o OrderTerm[widget]

	c, ok := any(o.Column).(AnyColumn[widget])
	if !ok {
		t.Fatalf("OrderTerm.Column must be AnyColumn[widget], got %T", o.Column)
	}

	if c.Name() != "" {
		t.Fatalf("zero OrderTerm must carry an empty column ref, got %q", c.Name())
	}

	var a Assignment[widget]

	if _, ok := any(a.Column).(AnyColumn[widget]); !ok {
		t.Fatalf("Assignment.Column must be AnyColumn[widget], got %T", a.Column)
	}
}
