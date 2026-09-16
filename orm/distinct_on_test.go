package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// TestQueryDistinctOnRenders proves Query.DistinctOn renders the Postgres
// `SELECT DISTINCT ON (cols)` prefix and keeps ORDER BY for the deterministic
// first-row-per-group semantics.
func TestQueryDistinctOnRenders(t *testing.T) {
	ctx := context.Background()

	rec := &ormRecordingExec{dialectName: "postgres"}

	if _, err := From(widgets).
		DistinctOn(widgetName.Col()).
		OrderBy(widgetID.Asc()).
		All(ctx, rec); err != nil {
		t.Fatalf("All: %v", err)
	}

	q, _ := rec.last()

	if !strings.Contains(q, `SELECT DISTINCT ON ("name")`) {
		t.Fatalf("query %q missing DISTINCT ON", q)
	}

	if !strings.Contains(q, `ORDER BY "id" ASC`) {
		t.Fatalf("query %q missing ORDER BY", q)
	}
}

// TestQueryDistinctOnCapabilityGate proves DISTINCT ON is a typed
// dialect.ErrUnsupportedByDialect on every dialect without the capability --
// including a base-only dialect with no capability sub-interfaces at all.
func TestQueryDistinctOnCapabilityGate(t *testing.T) {
	ctx := context.Background()

	for _, name := range []string{"sqlite", "mock-nocap"} {
		t.Run(name, func(t *testing.T) {
			_, err := From(widgets).DistinctOn(widgetName.Col()).All(ctx, mockExec{dialectName: name})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}
}

// TestQueryDistinctOnWithLockRejected proves DISTINCT ON participates in the
// existing DISTINCT + row-lock guard.
func TestQueryDistinctOnWithLockRejected(t *testing.T) {
	ctx := context.Background()

	_, err := From(widgets).DistinctOn(widgetName.Col()).ForUpdate().All(ctx, mockExec{dialectName: "postgres"})
	if !errors.Is(err, ErrLockingWithDistinct) {
		t.Fatalf("err = %v, want errors.Is(err, ErrLockingWithDistinct)", err)
	}
}

// TestQueryDistinctOnEmptyRejected proves a zero-column DISTINCT ON is a
// typed caller error rather than an invalid `DISTINCT ON ()`.
func TestQueryDistinctOnEmptyRejected(t *testing.T) {
	ctx := context.Background()

	_, err := From(widgets).DistinctOn().All(ctx, mockExec{dialectName: "postgres"})
	if err == nil {
		t.Fatalf("err = nil, want an error for an empty DISTINCT ON list")
	}
}

// TestQueryDistinctOnBranchSafe proves DistinctOn is copy-on-write.
func TestQueryDistinctOnBranchSafe(t *testing.T) {
	base := From(widgets)

	branch := base.DistinctOn(widgetName.Col())

	if base.distinctOn != nil {
		t.Fatalf("base.distinctOn = %v after branching, want nil", base.distinctOn)
	}

	if len(branch.distinctOn) != 1 || branch.distinctOn[0] != "name" {
		t.Fatalf("branch.distinctOn = %v, want [name]", branch.distinctOn)
	}
}
