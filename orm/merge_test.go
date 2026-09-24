package orm

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// stagingWidget is the source-side fixture for MERGE tests: a staging table
// whose columns feed the target's WHEN MATCHED UPDATE and WHEN NOT MATCHED
// INSERT clauses.
type stagingWidget struct {
	WidgetID string
	Name     string
}

var (
	stagingWidgets = NewTable[stagingWidget]("widget_staging", []string{"widget_id", "name"})
	stagingID      = NewColumn[stagingWidget, string]("widget_staging", "widget_id")
	stagingName    = NewColumn[stagingWidget, string]("widget_staging", "name")
)

// TestMergeExecPostgresRendersSQL proves the typed Merge builder renders the
// SQL-standard MERGE and executes it on Postgres, binding only literal
// values (source-column references bind nothing).
func TestMergeExecPostgresRendersSQL(t *testing.T) {
	ctx := t.Context()

	rec := &recordingMock{dialectName: "postgres"}

	m := MergeInto(widgets).
		UsingSource(stagingWidgets).
		On(widgetID.Col(), stagingID.Col()).
		WhenMatchedUpdate(
			MergeSource(widgetName, stagingName),
			MergeValue(widgetQty.Col(), int64(7)),
		).
		WhenNotMatchedInsert(
			MergeSource(widgetID, stagingID),
			MergeSource(widgetName, stagingName),
		)

	if _, err := m.Exec(ctx, rec); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	want := `MERGE INTO "widgets" USING "widget_staging" ` +
		`ON "widgets"."id" = "widget_staging"."widget_id" ` +
		`WHEN MATCHED THEN UPDATE SET "name" = "widget_staging"."name", "quantity" = $1 ` +
		`WHEN NOT MATCHED THEN INSERT ("id", "name") VALUES ("widget_staging"."widget_id", "widget_staging"."name")`

	if rec.lastQuery != want {
		t.Fatalf("rendered:\n%q\nwant:\n%q", rec.lastQuery, want)
	}

	if !reflect.DeepEqual(rec.lastArgs, []any{int64(7)}) {
		t.Fatalf("args = %#v, want [7]", rec.lastArgs)
	}
}

// TestMergeExecPostgresMatchedDelete proves a MERGE with a WHEN MATCHED THEN
// DELETE clause renders and runs.
func TestMergeExecPostgresMatchedDelete(t *testing.T) {
	ctx := t.Context()

	rec := &recordingMock{dialectName: "postgres"}

	if _, err := MergeInto(widgets).
		UsingSource(stagingWidgets).
		On(widgetID.Col(), stagingID.Col()).
		WhenMatchedDelete().
		Exec(ctx, rec); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if !strings.Contains(rec.lastQuery, "WHEN MATCHED THEN DELETE") {
		t.Fatalf("rendered %q, want WHEN MATCHED THEN DELETE", rec.lastQuery)
	}
}

// TestMergeShapeMismatchProvenByBranchSafety proves the builder follows the
// same copy-on-write discipline as every other orm chain: branching a base
// Merge and adding a clause on the branch leaves the base untouched.
func TestMergeBranchSafety(t *testing.T) {
	ctx := t.Context()

	base := MergeInto(widgets).UsingSource(stagingWidgets).On(widgetID.Col(), stagingID.Col())

	recA := &recordingMock{dialectName: "postgres"}
	recB := &recordingMock{dialectName: "postgres"}

	a := base.WhenMatchedDelete()
	b := base.WhenNotMatchedInsert(MergeSource(widgetID, stagingID))

	if _, err := a.Exec(ctx, recA); err != nil {
		t.Fatalf("a.Exec: %v", err)
	}

	if _, err := b.Exec(ctx, recB); err != nil {
		t.Fatalf("b.Exec: %v", err)
	}

	if !strings.Contains(recA.lastQuery, "WHEN MATCHED THEN DELETE") {
		t.Fatalf("branch a rendered %q, want DELETE", recA.lastQuery)
	}

	if !strings.Contains(recB.lastQuery, "WHEN NOT MATCHED THEN INSERT") {
		t.Fatalf("branch b rendered %q, want INSERT", recB.lastQuery)
	}
}

// TestMergeCapabilityGate proves MERGE is rejected with the typed
// dialect.ErrUnsupportedByDialect on every dialect that has no MERGE --
// SQLite and a base-only dialect -- never rendered as invalid SQL.
func TestMergeCapabilityGate(t *testing.T) {
	ctx := t.Context()

	m := MergeInto(widgets).
		UsingSource(stagingWidgets).
		On(widgetID.Col(), stagingID.Col()).
		WhenMatchedDelete()

	for _, name := range []string{"sqlite", "mock-nocap"} {
		t.Run(name, func(t *testing.T) {
			_, err := m.Exec(ctx, mockExec{dialectName: name})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("Merge.Exec on %s = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", name, err)
			}
		})
	}
}

// TestMergeFailsClosed proves a malformed Merge (no ON, no WHEN) surfaces an
// error rather than rendering invalid SQL.
func TestMergeFailsClosed(t *testing.T) {
	ctx := t.Context()

	rec := &recordingMock{dialectName: "postgres"}

	_, err := MergeInto(widgets).UsingSource(stagingWidgets).Exec(ctx, rec)
	if err == nil {
		t.Fatalf("Merge without ON/WHEN succeeded, want an error")
	}

	_, err = MergeInto(widgets).UsingSource(stagingWidgets).On(widgetID.Col(), stagingID.Col()).Exec(ctx, rec)
	if err == nil {
		t.Fatalf("Merge without any WHEN succeeded, want an error")
	}
}

// TestMergeExecErrorPaths drives resolve and exec-layer failures through
// Merge.Exec.
func TestMergeExecErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	m := MergeInto(widgets).
		UsingSource(stagingWidgets).
		On(widgetID.Col(), stagingID.Col()).
		WhenMatchedDelete()

	if _, err := m.Exec(ctx, fakeDB{}); err == nil {
		t.Fatal("Exec on an unresolvable dialect succeeded, want an error")
	}

	stub := &ormStubDB{mockExec: mockExec{dialectName: "postgres"}, execErr: boom}

	if _, err := m.Exec(ctx, stub); !errors.Is(err, boom) {
		t.Fatalf("Exec err = %v, want errors.Is(err, boom)", err)
	}
}
