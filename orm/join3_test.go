package orm

import (
	"testing"
)

// TestJoin3InnerAllScansThreeSides proves an all-INNER Join3 via JoinOn3
// returns a typed []Row3[A, B, C] with correct field values from ALL THREE
// tables, via a real SQLite in-memory database. Seed: u1 has 3 orders
// (o1: 2 items, o2: 1 item, o3: none) -- so the users→orders→items chain
// yields exactly 3 rows.
func TestJoin3InnerAllScansThreeSides(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	for _, r := range rows {
		if r.A.ID != "u1" {
			t.Fatalf("row A.ID = %q, want u1", r.A.ID)
		}

		if r.B.UserID != "u1" {
			t.Fatalf("row B.UserID = %q, want u1", r.B.UserID)
		}

		if r.C.OrderID == "o3" {
			t.Fatalf("row C.OrderID = o3, want an item-bearing order (o1/o2)")
		}
	}

	skus := map[string]bool{}
	for _, r := range rows {
		skus[r.C.SKU] = true
	}

	for _, want := range []string{"pen", "paper", "pencil"} {
		if !skus[want] {
			t.Fatalf("missing item %q among %v", want, skus)
		}
	}
}

// TestJoin3WhereRightAndWhereC proves Where (left), WhereRight (middle)
// and WhereC (rightmost) all narrow the three-table joined result.
func TestJoin3WhereRightAndWhereC(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		WhereRight(joinOrderCents.Gt(100)).
		WhereC(joinItemSKU.Eq("pencil")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// amount > 100 keeps o2 (200) and o3 (300); sku = pencil only exists on
	// o2's item i3.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	if rows[0].B.ID != "o2" || rows[0].C.ID != "i3" {
		t.Fatalf("row = %+v, want B=o2 C=i3", rows[0])
	}
}

// TestJoin3OrderByRightAndOrderByC proves Join3's right-table-aware
// ordering: OrderByRight orders by B, OrderByC by C, on top of the
// left-scoped OrderBy.
func TestJoin3OrderByRightAndOrderByC(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).
		OrderByRight(joinOrderCents.Desc()).
		OrderByC(joinItemSKU.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	// Amounts descending: o2 (200) first, then o1's two items (100 each).
	if rows[0].B.AmountCents != 200 {
		t.Fatalf("rows[0].B.AmountCents = %d, want 200 (o2 ordered first)", rows[0].B.AmountCents)
	}

	if rows[1].C.SKU != "paper" || rows[2].C.SKU != "pen" {
		t.Fatalf("o1's items ordered %q,%q, want paper,pen (sku ascending)", rows[1].C.SKU, rows[2].C.SKU)
	}
}

// TestJoin3Stream proves Join3.Stream yields the same rows All does.
func TestJoin3Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var got []Row3[joinUser, joinOrder, joinOrderItem]

	for row, err := range JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		got = append(got, row)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
}

// TestJoin3IsSingleRoundTrip proves Join3.All issues exactly one query for
// the whole three-table chain -- a single-round-trip join, never N+1.
func TestJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("Join3.All issued %d queries, want exactly 1", counting.queries)
	}
}
