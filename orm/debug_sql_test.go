package orm

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// debugDialectSeq mints unique registry names: registering a fixed name
// would collide across go test -count=N reruns in one process.
var debugDialectSeq atomic.Uint64

// TestQueryDebugSQLMatchesStream proves DebugSQL renders exactly the SQL
// Stream executes: same text, same placeholders, same encoded args.
func TestQueryDebugSQLMatchesStream(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	stmts, err := From(widgets).Where(widgetID.Eq("w1")).OrderBy(widgetID.Asc()).Limit(10).DebugSQL("sqlite")
	if err != nil {
		t.Fatalf("DebugSQL: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("DebugSQL returned %d statements, want 1", len(stmts))
	}

	var got []loggedQuery

	restore := collectLogs(&got)
	defer restore()

	if _, err := From(widgets).Where(widgetID.Eq("w1")).OrderBy(widgetID.Asc()).Limit(10).All(ctx, conn); err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("Stream issued %d statements, want 1", len(got))
	}

	if stmts[0].SQL != got[0].query {
		t.Fatalf("DebugSQL SQL = %q, Stream issued %q", stmts[0].SQL, got[0].query)
	}

	if fmt.Sprint(stmts[0].Args) != fmt.Sprint(got[0].args) {
		t.Fatalf("DebugSQL args = %v, Stream issued %v", stmts[0].Args, got[0].args)
	}
}

// TestQueryDebugSQLPostgresPlaceholders proves DebugSQL renders the named
// dialect without a live connection: postgres $n placeholders here.
func TestQueryDebugSQLPostgresPlaceholders(t *testing.T) {
	stmts, err := From(widgets).Where(widgetID.Eq("w1")).DebugSQL("postgres")
	if err != nil {
		t.Fatalf("DebugSQL: %v", err)
	}

	if len(stmts) != 1 || !strings.Contains(stmts[0].SQL, `"id" = $1`) {
		t.Fatalf("SQL = %q, want postgres $1 placeholder", stmts[0].SQL)
	}
}

// TestQueryDebugSQLErrors covers the fail-closed paths: unknown dialect,
// unsupported modifier, and the DebugSQL error tag on an oversized IN
// under ORDER BY (which cannot fan out).
func TestQueryDebugSQLErrors(t *testing.T) {
	if _, err := From(widgets).DebugSQL("no-such-dialect"); err == nil {
		t.Fatal("DebugSQL with unknown dialect succeeded, want an error")
	} else if !errors.Is(err, dialect.ErrUnknownDialect) || !strings.Contains(err.Error(), "Query.DebugSQL") {
		t.Fatalf("err = %v, want ErrUnknownDialect wrapped with the Query.DebugSQL tag", err)
	}

	if _, err := From(widgets).ForUpdate().DebugSQL("sqlite"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	// A pre-3.32 SQLite budgets 999 params: 1001 IN values fan out, and
	// ORDER BY forbids fan-out, so DebugSQL fails with the chunk error.
	old, err := sqlite.NewWithVersion("3.31.0")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	name := fmt.Sprintf("debug-tiny-%d", debugDialectSeq.Add(1))
	if err = dialect.Register(name, func() dialect.Dialect { return old }); err != nil {
		t.Fatalf("Register: %v", err)
	}

	vs := make([]string, 1001)
	for i := range vs {
		vs[i] = fmt.Sprintf("w%d", i)
	}

	_, err = From(widgets).Where(widgetID.In(vs...)).OrderBy(widgetID.Asc()).DebugSQL(name)
	if !errors.Is(err, ErrInListTooLarge) {
		t.Fatalf("err = %v, want errors.Is(err, ErrInListTooLarge)", err)
	}
}
