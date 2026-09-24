package orm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// lockingForms enumerates the public locking modifier chains exercised by
// the capability gates -- one row per distinct SQL suffix form.
var lockingForms = []struct {
	name  string
	apply func(Query[widget, *widget]) Query[widget, *widget]
}{
	{"ForUpdate", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForUpdate() }},
	{"ForShare", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForShare() }},
	{"ForUpdateNoWait", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForUpdate().NoWait() }},
	{"ForUpdateSkipLocked", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForUpdate().SkipLocked() }},
	{"ForShareNoWait", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForShare().NoWait() }},
	{"ForShareSkipLocked", func(q Query[widget, *widget]) Query[widget, *widget] { return q.ForShare().SkipLocked() }},
}

// TestLockingCapabilityTruthTable pins the empirically verified locking
// capability matrix:
//
// - Postgres: FOR UPDATE, FOR SHARE, NOWAIT and SKIP LOCKED are all
// supported.
// - SQLite: no row-level locking at any version, so every method reports
// false and every request is a typed dialect.ErrUnsupportedByDialect.
func TestLockingCapabilityTruthTable(t *testing.T) {
	var (
		_ dialect.LockingDialect = sqlite.New()
		_ dialect.LockingDialect = postgres.New()
	)

	s := sqlite.New()
	if s.SupportsForUpdate() || s.SupportsForShare() || s.SupportsNoWait() || s.SupportsSkipLocked() {
		t.Fatalf("sqlite locking capabilities = (%v, %v, %v, %v), want all false",
			s.SupportsForUpdate(), s.SupportsForShare(), s.SupportsNoWait(), s.SupportsSkipLocked())
	}

	p := postgres.New()
	if !p.SupportsForUpdate() || !p.SupportsForShare() || !p.SupportsNoWait() || !p.SupportsSkipLocked() {
		t.Fatalf("postgres locking capabilities = (%v, %v, %v, %v), want all true",
			p.SupportsForUpdate(), p.SupportsForShare(), p.SupportsNoWait(), p.SupportsSkipLocked())
	}
}

// TestLockingCapabilityGate drives every locking form through SQLite, a
// base-only dialect (no LockingDialect at all), and Postgres. SQLite and
// mock-nocap must reject with the typed dialect.ErrUnsupportedByDialect;
// Postgres must pass the gate and (with mockExec's empty result set) return
// no rows rather than an error.
func TestLockingCapabilityGate(t *testing.T) {
	ctx := t.Context()

	for _, tc := range lockingForms {
		for _, d := range []string{"sqlite", "mock-nocap"} {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				_, err := tc.apply(From(widgets)).All(ctx, mockExec{dialectName: d})
				if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
					t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
				}
			})
		}

		t.Run(tc.name+"/postgres", func(t *testing.T) {
			if _, err := tc.apply(From(widgets)).All(ctx, mockExec{dialectName: "postgres"}); err != nil {
				t.Fatalf("err = %v, want nil (postgres supports every locking form)", err)
			}
		})
	}
}
