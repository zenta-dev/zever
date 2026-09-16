package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// TestInnerLeftJoin3AllScansThreeSides is the INNER-then-LEFT proof test:
// over the newJoinDB fixture a chain "A INNER JOIN B ON ... LEFT JOIN C ON
// ..." keeps exactly the (user, order) pairs an inner join produces -- u1's
// three orders, u2's absence dropped -- and makes C optional, so o3 (no
// items) survives with C = None while o1/o2 keep their items.
//
// It also proves B is NEVER optional in this chain: A INNER JOIN B means
// every surviving row has a real B, so B is returned unwrapped.
func TestInnerLeftJoin3AllScansThreeSides(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		OrderByC(joinItemSKU.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1's orders o1 (2 items), o2 (1 item), o3 (0 items) => 4 rows; u2 has
	// no orders and is dropped by the INNER join.
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	var (
		cSome int
		cNone int
	)

	for _, r := range rows {
		if r.A.ID != "u1" {
			t.Fatalf("row A.ID = %q, want u1 (u2 must be dropped by the INNER join)", r.A.ID)
		}

		if r.B.ID == "" {
			t.Fatalf("B came back zero-valued on an INNER-joined B: %+v", r)
		}

		if r.C.IsSome() {
			cSome++
		} else {
			cNone++

			if r.B.ID != "o3" {
				t.Fatalf("C = None on B = %q, want only o3 (the order with no items)", r.B.ID)
			}
		}
	}

	if cSome != 3 {
		t.Fatalf("C Some = %d, want 3", cSome)
	}

	if cNone != 1 {
		t.Fatalf("C None = %d, want 1 (order o3)", cNone)
	}

	if rows[3].C.IsSome() {
		t.Fatalf("last row C = Some(%+v), want None for o3", rows[3].C.GetOr(joinOrderItem{}))
	}
}

// TestInnerLeftJoin3WherePredicates proves Where (A), WhereRight (B) and
// WhereC (C) all narrow an INNER-then-LEFT chain.
func TestInnerLeftJoin3WherePredicates(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		Where(joinUserMail.Eq("a@example.com")).
		WhereRight(joinOrderCents.Gt(100)).
		WhereC(joinItemSKU.Eq("pencil")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 only, amount > 100 keeps o2 (200) and o3 (300), sku = pencil only
	// exists on o2's item i3.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	if rows[0].B.ID != "o2" || rows[0].C.GetOr(joinOrderItem{}).ID != "i3" {
		t.Fatalf("row = %+v, want B=o2 C=i3", rows[0])
	}
}

// TestInnerLeftJoin3OrderByRightAndOrderByC proves the mixed builder keeps
// the per-table ordering split: OrderByRight orders by B, OrderByC by C, on
// top of the left-scoped OrderBy.
func TestInnerLeftJoin3OrderByRightAndOrderByC(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Desc()).
		OrderByC(joinItemSKU.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	// Amounts descending: o3 (300, C None) first, then o2 (200, pencil),
	// then o1's two items (100 each: paper then pen ascending).
	if rows[0].B.ID != "o3" || rows[0].C.IsSome() {
		t.Fatalf("rows[0] = %+v, want o3 with C None", rows[0])
	}

	if rows[1].B.ID != "o2" || rows[1].C.GetOr(joinOrderItem{}).SKU != "pencil" {
		t.Fatalf("rows[1] = %+v, want o2/pencil", rows[1])
	}

	if rows[2].C.GetOr(joinOrderItem{}).SKU != "paper" || rows[3].C.GetOr(joinOrderItem{}).SKU != "pen" {
		t.Fatalf("o1 items ordered %q,%q, want paper,pen",
			rows[2].C.GetOr(joinOrderItem{}).SKU, rows[3].C.GetOr(joinOrderItem{}).SKU)
	}
}

// TestInnerLeftJoin3Limit proves Limit is honoured on the mixed chain.
func TestInnerLeftJoin3Limit(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		OrderByC(joinItemSKU.Asc()).
		Limit(2).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	for i, r := range rows {
		if r.B.ID != "o1" {
			t.Fatalf("rows[%d].B.ID = %q, want o1 (lowest amount first)", i, r.B.ID)
		}
	}
}

// TestInnerLeftJoin3Offset proves Offset skips leading rows on the mixed
// chain: with the same deterministic order the first two rows are o1's two
// items, so an offset of 2 starts at o2 (pencil), then o3 (C None). A Limit
// accompanies it because SQLite (like the shared SELECT renderer) requires
// LIMIT before OFFSET.
func TestInnerLeftJoin3Offset(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		OrderByC(joinItemSKU.Asc()).
		Limit(10).
		Offset(2).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	if rows[0].B.ID != "o2" || rows[0].C.GetOr(joinOrderItem{}).SKU != "pencil" {
		t.Fatalf("rows[0] = %+v, want o2/pencil", rows[0])
	}

	if rows[1].B.ID != "o3" || rows[1].C.IsSome() {
		t.Fatalf("rows[1] = %+v, want o3 with C None", rows[1])
	}
}

// TestInnerLeftJoin3Stream proves InnerLeftJoin3.Stream yields the same
// Option-wrapped rows All does, including the C None case.
func TestInnerLeftJoin3Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var (
		cSome int
		cNone int
	)

	for row, err := range InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.B.ID == "" {
			t.Fatalf("B came back zero-valued on an INNER-joined B: %+v", row)
		}

		if row.C.IsSome() {
			cSome++
		} else {
			cNone++
		}
	}

	if cSome != 3 || cNone != 1 {
		t.Fatalf("Stream cSome=%d cNone=%d, want 3/1", cSome, cNone)
	}
}

// TestInnerLeftJoin3IsSingleRoundTrip proves a mixed chain still issues
// exactly one query -- never N+1.
func TestInnerLeftJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("InnerLeftJoin3.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestInnerLeftJoin3IsSingleScanPerRow proves each returned row is populated
// with exactly ONE underlying rows.Scan call, however the optional side
// scans.
func TestInnerLeftJoin3IsSingleScanPerRow(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &scanCountingDB{DB: conn}

	rows, err := InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if counting.scans != len(rows) {
		t.Fatalf("underlying Scan calls = %d, want %d (one per row)", counting.scans, len(rows))
	}
}

// TestLeftInnerJoin3BAndCFormOptionalUnit is THE LEFT-then-INNER proof test,
// and the one that pins the SQL shape: "A LEFT JOIN B INNER JOIN C" must
// render as A LEFT JOIN (B INNER JOIN C ON ...) ON ... -- a parenthesized
// composite -- NOT as the left-associative flat chain, which would silently
// drop every A row without a B (here user u2).
//
// With that shape A is always present, and B/C come as an all-or-nothing
// unit: B Some <=> C Some. The brief asks for the weaker documented
// invariant C Some => B Some (and B None => C None); both directions are
// asserted here against real SQLite SQL.
func TestLeftInnerJoin3BAndCFormOptionalUnit(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1: (o1,i1), (o1,i2), (o2,i3) => 3 rows with B and C Some; u2 has no
	// order with an item => 1 row with B None and C None. A flat render
	// would return only 3 rows (u2 dropped).
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4 (u2 must survive with B/C None)", len(rows))
	}

	var (
		bothSome int
		bothNone int
	)

	for _, r := range rows {
		// The documented invariant: a present C always implies a present B.
		if r.C.IsSome() && !r.B.IsSome() {
			t.Fatalf("row has C = Some but B = None, violating C Some => B Some: %+v", r)
		}

		// The stronger equivalence the parenthesized composite actually
		// guarantees: B and C are present together or absent together.
		if r.B.IsSome() != r.C.IsSome() {
			t.Fatalf("row has B Some = %v but C Some = %v, want both-or-neither: %+v", r.B.IsSome(), r.C.IsSome(), r)
		}

		if r.B.IsSome() && r.C.IsSome() {
			bothSome++
		} else {
			bothNone++

			if r.A.ID != "u2" {
				t.Fatalf("B/C None row has A.ID = %q, want u2", r.A.ID)
			}
		}
	}

	if bothSome != 3 {
		t.Fatalf("bothSome = %d, want 3", bothSome)
	}

	if bothNone != 1 {
		t.Fatalf("bothNone = %d, want 1 (u2)", bothNone)
	}
}

// TestLeftInnerJoin3WherePredicates proves Where (A), WhereRight (B) and
// WhereC (C) all narrow a LEFT-then-INNER chain. Filtering an optional side
// in WHERE necessarily discards None rows, the same caveat documented on
// LeftJoin2.WhereRight.
func TestLeftInnerJoin3WherePredicates(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		Where(joinUserMail.Eq("a@example.com")).
		WhereRight(joinOrderCents.Gt(100)).
		WhereC(joinItemSKU.Eq("pencil")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 only, amount > 100 keeps o2 (200), sku = pencil is o2's item i3.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	if rows[0].B.GetOr(joinOrder{}).ID != "o2" || rows[0].C.GetOr(joinOrderItem{}).ID != "i3" {
		t.Fatalf("row = %+v, want B=o2 C=i3", rows[0])
	}
}

// TestLeftInnerJoin3Limit proves Limit is honoured on a LEFT-then-INNER
// chain.
func TestLeftInnerJoin3Limit(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		Limit(2).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	for i, r := range rows {
		if !r.B.IsSome() || !r.C.IsSome() {
			t.Fatalf("rows[%d] = %+v, want both B and C Some", i, r)
		}
	}
}

// TestLeftInnerJoin3Offset proves Offset skips leading rows on the nested
// chain: ordered by user then order amount, the first two rows are u1/o1's
// two items, so an offset of 2 starts at u1/o2 and then u2's both-None row.
// A Limit accompanies it because SQLite (like the shared SELECT renderer)
// requires LIMIT before OFFSET.
func TestLeftInnerJoin3Offset(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		Limit(10).
		Offset(2).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	if rows[0].B.GetOr(joinOrder{}).ID != "o2" || !rows[0].C.IsSome() {
		t.Fatalf("rows[0] = %+v, want u1/o2 with C Some", rows[0])
	}

	if rows[1].A.ID != "u2" || rows[1].B.IsSome() || rows[1].C.IsSome() {
		t.Fatalf("rows[1] = %+v, want u2 with B/C None", rows[1])
	}
}

// TestLeftInnerJoin3Stream proves LeftInnerJoin3.Stream yields the same rows
// All does, including the both-None case.
func TestLeftInnerJoin3Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var bothSome, bothNone int

	for row, err := range LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.C.IsSome() && !row.B.IsSome() {
			t.Fatalf("row has C = Some but B = None: %+v", row)
		}

		if row.B.IsSome() && row.C.IsSome() {
			bothSome++
		} else {
			bothNone++
		}
	}

	if bothSome != 3 || bothNone != 1 {
		t.Fatalf("Stream bothSome=%d bothNone=%d, want 3/1", bothSome, bothNone)
	}
}

// TestLeftInnerJoin3IsSingleRoundTrip proves the nested mixed chain still
// issues exactly one query.
func TestLeftInnerJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("LeftInnerJoin3.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestLeftInnerJoin3IsSingleScanPerRow proves each returned row is populated
// with exactly ONE underlying rows.Scan call.
func TestLeftInnerJoin3IsSingleScanPerRow(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &scanCountingDB{DB: conn}

	rows, err := LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if counting.scans != len(rows) {
		t.Fatalf("underlying Scan calls = %d, want %d (one per row)", counting.scans, len(rows))
	}
}

// TestMixedJoin3UngatedOnBaseDialect proves both mixed INNER/LEFT chains are
// universal SQL and never gated: on a dialect implementing only the base
// dialect.Dialect interface (no JoinCapabilities) All and Stream succeed
// rather than returning ErrUnsupportedByDialect.
func TestMixedJoin3UngatedOnBaseDialect(t *testing.T) {
	ctx := context.Background()

	left := From[joinUser](joinUsers)

	if _, err := InnerLeftJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("InnerLeftJoin3 on a base-only dialect failed: %v (INNER/LEFT are universal SQL)", err)
	}

	if _, err := LeftInnerJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("LeftInnerJoin3 on a base-only dialect failed: %v (INNER/LEFT are universal SQL)", err)
	}

	for _, err := range InnerLeftJoinOn3(left, userOrdersRel, orderItemsRel).Stream(ctx, mockExec{dialectName: "mock-nocap"}) {
		if err != nil {
			t.Fatalf("InnerLeftJoin3.Stream on a base-only dialect failed: %v", err)
		}
	}

	for _, err := range LeftInnerJoinOn3(left, userOrdersRel, orderItemsRel).Stream(ctx, mockExec{dialectName: "mock-nocap"}) {
		if err != nil {
			t.Fatalf("LeftInnerJoin3.Stream on a base-only dialect failed: %v", err)
		}
	}
}

// TestMixedJoinStreamErrorPaths drives resolve, query, scan, iteration and
// close failures through the shared three-table stream tail.
func TestMixedJoinStreamErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	newMixed := func() MixedJoin3[joinUser, *joinUser, joinOrder, *joinOrder, joinOrderItem, *joinOrderItem] {
		return MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin)
	}

	if err := drainJoinStream(newMixed().Stream(ctx, fakeDB{})); err == nil {
		t.Fatal("Stream on an unresolvable dialect succeeded, want an error")
	}

	newStub := func(rows *stubRows) *ormStubDB {
		return &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: rows}
	}

	queryErr := newStub(&stubRows{})
	queryErr.queryErr = boom

	if err := drainJoinStream(newMixed().Stream(ctx, queryErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	scanErr := newStub(&stubRows{
		values:  [][]any{{"u1", "a@example.com", "o1", "u1", int64(100), "i1", "o1", "s1"}},
		scanErr: boom,
	})

	if err := drainJoinStream(newMixed().Stream(ctx, scanErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := newStub(&stubRows{
		values:  [][]any{{"u1", "a@example.com", "o1", "u1", int64(100), "i1", "o1", "s1"}},
		iterErr: boom,
	})

	if err := drainJoinStream(newMixed().Stream(ctx, iterErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := newStub(&stubRows{closeErr: boom})

	if err := drainJoinStream(newMixed().Stream(ctx, closeErr)); !errors.Is(err, boom) {
		t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
	}
}

// TestMixedJoinStreamRenderGate proves a gated-kind Stream yields the
// typed error instead of invalid SQL.
func TestMixedJoinStreamRenderGate(t *testing.T) {
	ctx := context.Background()

	j := MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, RightJoin, FullJoin)

	if err := drainJoinStream(j.Stream(ctx, mockExec{dialectName: "sqlite-3.38"})); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Stream err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}
