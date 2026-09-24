package orm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// recordingMock mirrors mockExec (see capability_test.go) but records the
// last Exec'd query text and bound arguments, so orm-level tests can prove
// exactly what SQL an Update/Delete chain renders against a chosen dialect
// name without a real database.
type recordingMock struct {
	dialectName string
	lastQuery   string
	lastArgs    []any
}

func (r *recordingMock) Query(context.Context, string, ...any) (db.Rows, error) {
	return emptyRows{}, nil
}
func (r *recordingMock) Exec(_ context.Context, q string, args ...any) (int64, error) {
	r.lastQuery = q
	r.lastArgs = args

	return 0, nil
}

func (*recordingMock) Ping(context.Context) error  { return nil }
func (*recordingMock) Close(context.Context) error { return nil }
func (r *recordingMock) Dialect() string           { return r.dialectName }

// TestUpdateOrderLimitBranchSafety proves OrderBy/Limit/Offset follow the
// copy-on-write chain discipline: branching a base Update and appending
// ORDER BY/LIMIT on the branch leaves the base (and a later sibling branch)
// untouched. The rendered statement is pinned through renderStatement
// against the postgres dialect (the ORDER BY/LIMIT capability gate lives at
// Exec, so rendering stays dialect-shaped here).
func TestUpdateOrderLimitBranchSafety(t *testing.T) {
	d := postgres.New()

	base := UpdateTable(widgets).Set(Set(widgetName, "Z"))

	a := base.OrderBy(widgetQty.Desc()).Limit(5)
	b := base.OrderBy(widgetID.Asc())

	q, args, err := a.renderStatement(d)
	if err != nil {
		t.Fatalf("a.renderStatement: %v", err)
	}

	if want := `UPDATE "widgets" SET "name" = $1 ORDER BY "quantity" DESC LIMIT $2`; q != want {
		t.Fatalf("a rendered %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"Z", 5}) {
		t.Fatalf("a args = %#v, want [\"Z\" 5]", args)
	}

	q, _, err = b.renderStatement(d)
	if err != nil {
		t.Fatalf("b.renderStatement: %v", err)
	}

	if want := `UPDATE "widgets" SET "name" = $1 ORDER BY "id" ASC`; q != want {
		t.Fatalf("b rendered %q, want %q", q, want)
	}

	q, _, err = base.renderStatement(d)
	if err != nil {
		t.Fatalf("base.renderStatement: %v", err)
	}

	if want := `UPDATE "widgets" SET "name" = $1`; q != want {
		t.Fatalf("base rendered %q, want %q (branch must not mutate the base)", q, want)
	}
}

func TestDeleteOrderLimitBranchSafety(t *testing.T) {
	d := postgres.New()

	base := DeleteFrom(widgets).Where(widgetID.Neq("w1"))

	a := base.OrderBy(widgetID.Asc()).Limit(3)
	b := base.OrderBy(widgetID.Desc())

	q, args, err := a.renderStatement(d)
	if err != nil {
		t.Fatalf("a.renderStatement: %v", err)
	}

	if want := `DELETE FROM "widgets" WHERE "id" != $1 ORDER BY "id" ASC LIMIT $2`; q != want {
		t.Fatalf("a rendered %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"w1", 3}) {
		t.Fatalf("a args = %#v, want [\"w1\" 3]", args)
	}

	q, _, err = b.renderStatement(d)
	if err != nil {
		t.Fatalf("b.renderStatement: %v", err)
	}

	if want := `DELETE FROM "widgets" WHERE "id" != $1 ORDER BY "id" DESC`; q != want {
		t.Fatalf("b rendered %q, want %q", q, want)
	}

	q, _, err = base.renderStatement(d)
	if err != nil {
		t.Fatalf("base.renderStatement: %v", err)
	}

	if want := `DELETE FROM "widgets" WHERE "id" != $1`; q != want {
		t.Fatalf("base rendered %q, want %q", q, want)
	}
}

// fullMutateDialect is a test-only dialect reporting every mutation
// capability as supported, so the success branches of the mutate-order and
// mutate-join gates are exercised without a live engine that accepts them
// (no in-tree dialect does).
type fullMutateDialect struct {
	mockDialect
}

func (fullMutateDialect) Name() string                         { return "mock-mutatefull" }
func (fullMutateDialect) SupportsUpdateJoin() bool             { return true }
func (fullMutateDialect) SupportsDeleteJoin() bool             { return true }
func (fullMutateDialect) SupportsLeftMutateJoin() bool         { return true }
func (fullMutateDialect) SupportsUpdateOrderLimit() bool       { return true }
func (fullMutateDialect) SupportsDeleteOrderLimit() bool       { return true }
func (fullMutateDialect) SupportsMutateOffset() bool           { return true }
func (fullMutateDialect) SupportsJoinedMutateOrderLimit() bool { return true }
func (fullMutateDialect) SupportsReturning() bool              { return true }

func init() {
	if err := dialect.Register("mock-mutatefull", func() dialect.Dialect { return fullMutateDialect{} }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration
	}
}

// TestMutateOrderSuccessOnFullCapability proves the gate's success branches:
// on a dialect reporting every mutation capability, single-table and joined
// ORDER BY/LIMIT/OFFSET updates and deletes run instead of failing closed.
func TestMutateOrderSuccessOnFullCapability(t *testing.T) {
	ctx := t.Context()
	e := mockExec{dialectName: "mock-mutatefull"}

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e); err != nil {
		t.Fatalf("update order-limit err = %v, want nil (full capability)", err)
	}

	if _, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Offset(1).Exec(ctx, e); err != nil {
		t.Fatalf("delete order-limit-offset err = %v, want nil (full capability)", err)
	}

	if _, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e); err != nil {
		t.Fatalf("joined update order-limit err = %v, want nil (full capability)", err)
	}

	if _, err := DeleteFrom(widgets).Join(rel, InnerJoin).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e); err != nil {
		t.Fatalf("joined delete order-limit err = %v, want nil (full capability)", err)
	}

	if _, err := UpdateTable(widgets).Join(rel, LeftJoin).Set(Set(widgetName, "x")).Exec(ctx, e); err != nil {
		t.Fatalf("joined update left join err = %v, want nil (full capability)", err)
	}

	if _, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetName, "x")).Returning(widgetID.Col()).OrderBy(widgetID.Asc()).Limit(2).ExecReturning[*widget](ctx, e); err != nil {
		t.Fatalf("joined update returning order-limit err = %v, want nil (full capability)", err)
	}

	if _, err := DeleteFrom(widgets).Join(rel, InnerJoin).Returning(widgetID.Col()).OrderBy(widgetID.Asc()).Limit(2).ExecReturning[*widget](ctx, e); err != nil {
		t.Fatalf("joined delete returning order-limit err = %v, want nil (full capability)", err)
	}
}

// Update/Delete through every dialect family and asserts the typed
// dialect.ErrUnsupportedByDialect -- never a panic or silently rendered
// SQL the engine would reject. Pinned against the empirically verified
// matrix: no in-tree dialect supports mutation ORDER BY/LIMIT/OFFSET;
// Postgres, SQLite, OFFSET, and every joined form are typed errors.
func TestMutateOrderCapabilityGate(t *testing.T) {
	ctx := t.Context()

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	updateCases := []struct {
		name string
		run  func(ctx context.Context, e db.DB) error
	}{
		{"order-limit/base-only", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"order-limit/postgres", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"order-limit/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"order-no-limit/postgres", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Desc()).Exec(ctx, e)

			return err
		}},
		{"limit-no-order/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Limit(2).Exec(ctx, e)

			return err
		}},
		{"offset/postgres", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Limit(2).Offset(1).Exec(ctx, e)

			return err
		}},
		{"offset-only/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Offset(1).Exec(ctx, e)

			return err
		}},
		{"join-order-limit/postgres", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"join-order-limit/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
	}

	for _, tc := range updateCases {
		t.Run("update/"+tc.name, func(t *testing.T) {
			err := tc.run(ctx, mockExec{dialectName: dialectName(tc.name)})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}

	deleteCases := []struct {
		name string
		run  func(ctx context.Context, e db.DB) error
	}{
		{"order-limit/base-only", func(ctx context.Context, e db.DB) error {
			_, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"order-limit/postgres", func(ctx context.Context, e db.DB) error {
			_, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"order-limit/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
		{"offset/postgres", func(ctx context.Context, e db.DB) error {
			_, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Offset(1).Exec(ctx, e)

			return err
		}},
		{"join-order-limit/sqlite", func(ctx context.Context, e db.DB) error {
			_, err := DeleteFrom(widgets).Join(rel, InnerJoin).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, e)

			return err
		}},
	}

	for _, tc := range deleteCases {
		t.Run("delete/"+tc.name, func(t *testing.T) {
			err := tc.run(ctx, mockExec{dialectName: dialectName(tc.name)})
			if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}

	// The zero-value convention (Limit(0)/Offset(0) render no clause) is
	// never gated.
	t.Run("update/sqlite limit-zero never gated", func(t *testing.T) {
		_, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Limit(0).Offset(0).Exec(ctx, mockExec{dialectName: "sqlite"})
		if err != nil {
			t.Fatalf("err = %v, want nil (Limit(0)/Offset(0) render no clause)", err)
		}
	})
}

// dialectName is a test helper returning the dialect name implied by a
// gate-test case label like "order-limit/postgres".
func dialectName(caseName string) string {
	names := []string{"mock-nocap", "postgres", "sqlite"}

	for _, n := range names {
		if strings.HasSuffix(caseName, "/"+n) {
			return n
		}
	}

	return "mock-nocap"
}

// TestMutateOrderUnsupportedOnRealSQLite proves the modernc sqlite driver
// (compiled without SQLITE_ENABLE_UPDATE_DELETE_LIMIT) rejects every
// mutation ORDER BY/LIMIT/OFFSET form with the typed error -- through the
// real adapter, not a mock -- while the same chains with Limit(0) or no
// order still run.
func TestMutateOrderUnsupportedOnRealSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, conn); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("update order-limit err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Limit(2).Exec(ctx, conn); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("update limit err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Offset(1).Exec(ctx, conn); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("update offset err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Exec(ctx, conn); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("delete order-limit err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	// The ExecReturning path composes the returning and mutate-order gates;
	// sqlite supports RETURNING but not mutation ORDER BY, so the typed
	// error still surfaces.
	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Returning(widgetID.Col()).OrderBy(widgetID.Asc()).Limit(2).ExecReturning[*widget](ctx, conn); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("update returning order-limit err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	// Zero limit/offset render no clause, so these plain statements still
	// run on real sqlite.
	n, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Limit(0).Offset(0).Exec(ctx, conn)
	if err != nil {
		t.Fatalf("update limit-zero err = %v, want nil", err)
	}

	if n != 3 {
		t.Fatalf("update limit-zero affected %d rows, want 3", n)
	}

	n, err = DeleteFrom(widgets).Where(widgetQty.Lt(int64(20))).Exec(ctx, conn)
	if err != nil {
		t.Fatalf("plain delete err = %v, want nil", err)
	}

	if n != 1 {
		t.Fatalf("plain delete affected %d rows, want 1", n)
	}
}

// partialMutateDialect reports mutation ORDER BY/LIMIT support without
// OFFSET (joined or single-table), so the OFFSET rejection branches are
// exercised without a live engine that draws that exact line.
type partialMutateDialect struct {
	mockDialect
}

func (partialMutateDialect) Name() string                   { return "mock-mutatepartial" }
func (partialMutateDialect) SupportsUpdateOrderLimit() bool { return true }
func (partialMutateDialect) SupportsDeleteOrderLimit() bool { return true }
func (partialMutateDialect) SupportsMutateOffset() bool     { return false }
func (partialMutateDialect) SupportsJoinedMutateOrderLimit() bool {
	return true
}
func (partialMutateDialect) SupportsUpdateJoin() bool     { return true }
func (partialMutateDialect) SupportsDeleteJoin() bool     { return true }
func (partialMutateDialect) SupportsLeftMutateJoin() bool { return true }

func init() {
	if err := dialect.Register("mock-mutatepartial", func() dialect.Dialect { return partialMutateDialect{} }); err != nil {
		panic(err) //nolint:forbidigo // test-only registration
	}
}

// TestMutateOffsetRejectedOnPartialCapability proves OFFSET fails closed
// on single-table and joined mutations when the dialect supports
// ORDER BY/LIMIT but not OFFSET.
func TestMutateOffsetRejectedOnPartialCapability(t *testing.T) {
	ctx := t.Context()
	e := mockExec{dialectName: "mock-mutatepartial"}

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Offset(1).Exec(ctx, e); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("update offset err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetName, "x")).OrderBy(widgetID.Asc()).Limit(2).Offset(1).Exec(ctx, e); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("joined update offset err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := DeleteFrom(widgets).OrderBy(widgetID.Asc()).Limit(2).Offset(1).Exec(ctx, e); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("delete offset err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}
