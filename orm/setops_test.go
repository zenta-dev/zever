package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// widgetIDs extracts the ordered ID list from a widget row slice.
func widgetIDs(rows []*widget) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}

	return out
}

// TestSetOpRoundTrips exercises every set operator against real SQLite over
// two overlapping widget subsets:
//
//	left = qty <= 20 -> {w1, w2}
//	right = qty >= 20 -> {w2, w3}
func TestSetOpRoundTrips(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	left := From(widgets).Where(widgetQty.Lte(20))
	right := From(widgets).Where(widgetQty.Gte(20))

	t.Run("union deduplicates", func(t *testing.T) {
		got, err := Union(left, right).OrderBy(widgetID.Asc()).All(ctx, conn)
		if err != nil {
			t.Fatalf("All: %v", err)
		}

		if ids := widgetIDs(got); len(ids) != 3 || ids[0] != "w1" || ids[1] != "w2" || ids[2] != "w3" {
			t.Fatalf("Union = %v, want [w1 w2 w3]", ids)
		}
	})

	t.Run("union all keeps duplicates", func(t *testing.T) {
		got, err := UnionAll(left, right).OrderBy(widgetID.Asc()).All(ctx, conn)
		if err != nil {
			t.Fatalf("All: %v", err)
		}

		if ids := widgetIDs(got); len(ids) != 4 || ids[0] != "w1" || ids[1] != "w2" || ids[2] != "w2" || ids[3] != "w3" {
			t.Fatalf("UnionAll = %v, want [w1 w2 w2 w3]", ids)
		}
	})

	t.Run("intersect", func(t *testing.T) {
		got, err := Intersect(left, right).All(ctx, conn)
		if err != nil {
			t.Fatalf("All: %v", err)
		}

		if ids := widgetIDs(got); len(ids) != 1 || ids[0] != "w2" {
			t.Fatalf("Intersect = %v, want [w2]", ids)
		}
	})

	t.Run("except", func(t *testing.T) {
		got, err := Except(left, right).All(ctx, conn)
		if err != nil {
			t.Fatalf("All: %v", err)
		}

		if ids := widgetIDs(got); len(ids) != 1 || ids[0] != "w1" {
			t.Fatalf("Except = %v, want [w1]", ids)
		}
	})

	t.Run("count", func(t *testing.T) {
		n, err := Union(left, right).Count(ctx, conn)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}

		if n != 3 {
			t.Fatalf("Count = %d, want 3", n)
		}
	})

	t.Run("first", func(t *testing.T) {
		row, ok, err := Union(left, right).OrderBy(widgetID.Asc()).First(ctx, conn)
		if err != nil {
			t.Fatalf("First: %v", err)
		}

		if !ok || row.ID != "w1" {
			t.Fatalf("First = (%v, %v), want w1", row, ok)
		}
	})

	t.Run("limit offset apply to the whole result", func(t *testing.T) {
		got, err := Union(left, right).OrderBy(widgetID.Asc()).Limit(1).Offset(1).All(ctx, conn)
		if err != nil {
			t.Fatalf("All: %v", err)
		}

		if ids := widgetIDs(got); len(ids) != 1 || ids[0] != "w2" {
			t.Fatalf("Limit(1).Offset(1) = %v, want [w2]", ids)
		}
	})
}

// TestSetOpQueryOrderByDoesNotShareBackingArray proves SetOpQuery.OrderBy
// follows the same copy-on-write branch-safety rule as Query[T, PT].
func TestSetOpQueryOrderByDoesNotShareBackingArray(t *testing.T) {
	base := Union(From(widgets), From(widgets)).OrderBy(widgetID.Asc())

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

	if branchA.order[1].Column.Name() == branchB.order[1].Column.Name() {
		t.Fatalf("branchA and branchB unexpectedly share an order entry")
	}
}

// TestSetOpExecutionErrorPaths drives resolve, render-gate, query, scan,
// iteration and close failures through SetOpQuery All/First/Count.
func TestSetOpExecutionErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	left := From(widgets).Where(widgetQty.Lte(20))
	right := From(widgets).Where(widgetQty.Gte(20))
	u := Union(left, right)

	if _, err := u.All(ctx, fakeDB{}); err == nil {
		t.Fatal("All on an unresolvable dialect succeeded, want an error")
	}

	if _, _, err := u.First(ctx, fakeDB{}); err == nil {
		t.Fatal("First on an unresolvable dialect succeeded, want an error")
	}

	if _, err := u.Count(ctx, fakeDB{}); err == nil {
		t.Fatal("Count on an unresolvable dialect succeeded, want an error")
	}

	if _, err := Intersect(left, right).All(ctx, mockExec{dialectName: "mock-nocap"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Intersect on mock-nocap err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	queryErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	if _, err := u.All(ctx, queryErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := u.Count(ctx, queryErr); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}

	scanErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{nil, nil, nil, nil}}, scanErr: boom}}

	if _, err := u.All(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{
		values:  [][]any{{"w1", "Alpha", int64(10), "first"}},
		iterErr: boom,
	}}

	if _, err := u.All(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{closeErr: boom}}

	if _, err := u.All(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	empty, ok, err := Union(From(widgets).Where(widgetQty.Gt(int64(1000))), From(widgets).Where(widgetQty.Gt(int64(2000)))).First(ctx, mockExec{dialectName: "sqlite"})
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if ok || empty != nil {
		t.Fatalf("First = (%v, %v), want (nil, false)", empty, ok)
	}
}

// TestSetOpCountErrorPaths drives render, scan, iteration and close
// failures through SetOpQuery.Count, plus the render-gate on All.
func TestSetOpCountErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	left := From(widgets).Where(widgetQty.Lte(20))
	right := From(widgets).Where(widgetQty.Gte(20))
	multi := From(widgetOrders)

	if _, err := Union(From(widgets).Where(widgetID.InSub(multi)), right).All(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("All with a multi-column IN subquery succeeded, want a render error")
	}

	if _, err := Union(From(widgets).Where(widgetID.InSub(multi)), right).Count(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("Count with a multi-column IN subquery succeeded, want a render error")
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: rows}
	}

	scanErr := newStub(&stubRows{values: [][]any{{int64(3)}}, scanErr: boom})

	if _, err := Union(left, right).Count(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{values: [][]any{{int64(3)}}, iterErr: boom})

	if _, err := Union(left, right).Count(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if _, err := Union(left, right).Count(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("Count err = %v, want errors.Is(err, boom)", err)
	}
}
