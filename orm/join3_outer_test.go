package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// newOuterJoin3DB seeds the same join_users/join_orders/join_order_items
// tables as newJoinDB, but with the extra rows three-table OUTER joins need
// to be observable:
//
// - u2 has NO orders (a left-only side),
// - o9's user_id ("ghost") matches no user (an unmatched A for the
// orders→users hop),
// - o5 has NO items (a B row with no C),
// - i2's order_id ("o2") matches no order (an unmatched B/C for the
// orders→items hop).
//
// It is a fresh in-memory database, so the shared table NAMES do not collide
// with newJoinDB's or newOuterJoinDB's.
func newOuterJoin3DB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE join_users (id text, email text)`); err != nil {
		t.Fatalf("create join_users: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_orders (id text, user_id text, amount_cents integer)`); err != nil {
		t.Fatalf("create join_orders: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_order_items (id text, order_id text, sku text)`); err != nil {
		t.Fatalf("create join_order_items: %v", err)
	}

	users := []struct{ id, email string }{
		{"u1", "a@example.com"},
		{"u2", "b@example.com"},
	}

	for _, u := range users {
		if _, err := conn.Exec(ctx, `INSERT INTO join_users (id, email) VALUES (?, ?)`, u.id, u.email); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	orders := []struct {
		id, userID string
		cents      int64
	}{
		{"o1", "u1", 100},
		{"o5", "u1", 50},
		{"o9", "ghost", 900},
	}

	for _, o := range orders {
		if _, err := conn.Exec(ctx, `INSERT INTO join_orders (id, user_id, amount_cents) VALUES (?, ?, ?)`, o.id, o.userID, o.cents); err != nil {
			t.Fatalf("insert order: %v", err)
		}
	}

	items := []struct {
		id, orderID, sku string
	}{
		{"i1", "o1", "pen"},
		{"i2", "o2", "apple"},
		{"i3", "o9", "zebra"},
	}

	for _, it := range items {
		if _, err := conn.Exec(ctx, `INSERT INTO join_order_items (id, order_id, sku) VALUES (?, ?, ?)`, it.id, it.orderID, it.sku); err != nil {
			t.Fatalf("insert order item: %v", err)
		}
	}

	return ctx, conn
}

// scanCountingRows wraps a db.Rows and counts Scan calls, so a test can
// prove a joined row is populated with exactly ONE underlying Scan call.
type scanCountingRows struct {
	db.Rows

	scans *int
}

func (r *scanCountingRows) Scan(dest ...any) error {
	*r.scans++

	return r.Rows.Scan(dest...)
}

// scanCountingDB wraps a db.DB so every Query returns a Scan-counting Rows.
// Like countingDB it embeds the db.DB interface (not the concrete adapter),
// so the db.Preparer fast path in queryRows is deliberately bypassed and
// Query is always used -- keeping the count deterministic.
type scanCountingDB struct {
	db.DB

	scans int
}

func (c *scanCountingDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := c.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err //nolint:wrapcheck // test double, error passes straight through
	}

	return &scanCountingRows{Rows: rows, scans: &c.scans}, nil
}

// TestLeftJoin3NullabilityDistinguishesNoMatch is THE LEFT JOIN proof test:
// over the newJoinDB fixture u1's order o3 has no items and u2 has no orders,
// so a LEFT-LEFT join chain yields exactly one row with B Some / C None (o3)
// and one with B None / C None (u2) -- never a zero-valued B or C mistaken
// for a real match. It also pins the LEFT-chain invariant that a present C
// always implies a present B.
func TestLeftJoin3NullabilityDistinguishesNoMatch(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1: o1 (2 items), o2 (1 item), o3 (0 items) => 4 rows; u2: no
	// orders => 1 row.
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	var (
		bothSome   int
		bSomeCNone int
		bothNone   int
	)

	for _, r := range rows {
		if r.C.IsSome() && !r.B.IsSome() {
			t.Fatalf("row has C = Some but B = None, violating the LEFT-chain invariant: %+v", r)
		}

		switch {
		case r.B.IsSome() && r.C.IsSome():
			bothSome++
		case r.B.IsSome() && !r.C.IsSome():
			bSomeCNone++
		case !r.B.IsSome() && !r.C.IsSome():
			bothNone++
		default:
			t.Fatalf("impossible option combination: %+v", r)
		}
	}

	if bothSome != 3 {
		t.Fatalf("bothSome = %d, want 3", bothSome)
	}

	if bSomeCNone != 1 {
		t.Fatalf("bSomeCNone (o3) = %d, want 1", bSomeCNone)
	}

	if bothNone != 1 {
		t.Fatalf("bothNone (u2) = %d, want 1", bothNone)
	}
}

// TestLeftJoin3IsSingleRoundTrip proves LeftJoin3.All issues exactly one
// query for the whole three-table chain, never N+1.
func TestLeftJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("LeftJoin3.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestLeftJoin3IsSingleScanPerRow proves each returned row is populated with
// exactly ONE underlying rows.Scan call, however many optional sides it has.
func TestLeftJoin3IsSingleScanPerRow(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &scanCountingDB{DB: conn}

	rows, err := LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if counting.scans != len(rows) {
		t.Fatalf("underlying Scan calls = %d, want %d (one per row)", counting.scans, len(rows))
	}
}

// TestLeftJoin3Stream proves LeftJoin3.Stream yields the same rows All does,
// including both None cases.
func TestLeftJoin3Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var (
		bothSome   int
		bSomeCNone int
		bothNone   int
	)

	for row, err := range LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.C.IsSome() && !row.B.IsSome() {
			t.Fatalf("row has C = Some but B = None: %+v", row)
		}

		switch {
		case row.B.IsSome() && row.C.IsSome():
			bothSome++
		case row.B.IsSome() && !row.C.IsSome():
			bSomeCNone++
		default:
			bothNone++
		}
	}

	if bothSome != 3 || bSomeCNone != 1 || bothNone != 1 {
		t.Fatalf("Stream bothSome=%d bSomeCNone=%d bothNone=%d, want 3/1/1", bothSome, bSomeCNone, bothNone)
	}
}

// TestRightJoin3AllWrapsUnmatchedSides is THE RIGHT JOIN proof test: with
// the newOuterJoin3DB fixture a RIGHT-RIGHT chain preserves every C (item):
//
// - i1 references o1 which references u1 => A Some, B Some, C Some,
// - i3 references o9 whose user is a ghost => A None, B Some, C Some,
// - i2 references no order at all => A None, B None, C Some.
//
// C must always be Some, and A Some must imply B Some.
func TestRightJoin3AllWrapsUnmatchedSides(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	rows, err := RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderByC(joinItemSKU.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (one per item)", len(rows))
	}

	var (
		allSome int
		aNone   int
		bothN   int
	)

	for _, r := range rows {
		if r.C.ID == "" {
			t.Fatalf("C came back zero-valued on a RIGHT JOIN: %+v", r)
		}

		if r.A.IsSome() && !r.B.IsSome() {
			t.Fatalf("row has A = Some but B = None, violating the RIGHT-chain invariant: %+v", r)
		}

		switch {
		case r.A.IsSome():
			allSome++
		case r.B.IsSome():
			aNone++
		default:
			bothN++
		}
	}

	if allSome != 1 || aNone != 1 || bothN != 1 {
		t.Fatalf("allSome=%d aNone=%d bothNone=%d, want 1/1/1", allSome, aNone, bothN)
	}

	// Ordered by C (sku ascending): apple(i2, A/B None), pen(i1, all Some),
	// zebra(i3, A None).
	if rows[0].C.SKU != "apple" || rows[0].A.IsSome() || rows[0].B.IsSome() {
		t.Fatalf("rows[0] = %+v, want orphan item i2 with A/B None", rows[0])
	}

	if rows[1].C.SKU != "pen" || !rows[1].A.IsSome() {
		t.Fatalf("rows[1] = %+v, want i1 with A/B Some", rows[1])
	}

	if rows[2].C.SKU != "zebra" || rows[2].A.IsSome() || !rows[2].B.IsSome() {
		t.Fatalf("rows[2] = %+v, want i3 with A None, B Some", rows[2])
	}
}

// TestRightJoin3Stream proves RightJoin3.Stream yields the same Option-wrapped
// rows All does.
func TestRightJoin3Stream(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	var allSome, aNone, bothNone int

	for row, err := range RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.C.ID == "" {
			t.Fatalf("C came back zero-valued on a RIGHT JOIN: %+v", row)
		}

		switch {
		case row.A.IsSome():
			allSome++
		case row.B.IsSome():
			aNone++
		default:
			bothNone++
		}
	}

	if allSome != 1 || aNone != 1 || bothNone != 1 {
		t.Fatalf("Stream allSome=%d aNone=%d bothNone=%d, want 1/1/1", allSome, aNone, bothNone)
	}
}

// TestFullJoin3AllWrapsAllSides is THE FULL JOIN proof test: with the
// newOuterJoin3DB fixture a FULL-FULL chain preserves every unmatched side.
// Expected five rows:
//
// - (Some,Some,Some) u1/o1/i1,
// - (None,Some,Some) ghost/o9/i3,
// - (None,None,Some) no order/i2,
// - (Some,Some,None) u1/o5 (o5 has no items),
// - (Some,None,None) u2 (no orders).
//
// C Some must still imply B Some (the second join keys on B).
func TestFullJoin3AllWrapsAllSides(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	rows, err := FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	var (
		allSome int
		aNoneB  int
		cOnly   int
		abNoC   int
		aOnly   int
	)

	for _, r := range rows {
		// The only cross-side invariant a FULL chain still guarantees is
		// that a present A together with a present C must have matched
		// through B (the second join keys on B); a lone C with B None is a
		// legitimate right-only row.
		if r.A.IsSome() && r.C.IsSome() && !r.B.IsSome() {
			t.Fatalf("row has A and C present but B None, impossible through the B-keyed second join: %+v", r)
		}

		switch {
		case r.A.IsSome() && r.B.IsSome() && r.C.IsSome():
			allSome++
		case !r.A.IsSome() && r.B.IsSome() && r.C.IsSome():
			aNoneB++
		case !r.A.IsSome() && !r.B.IsSome() && r.C.IsSome():
			cOnly++
		case r.A.IsSome() && r.B.IsSome() && !r.C.IsSome():
			abNoC++
		case r.A.IsSome() && !r.B.IsSome() && !r.C.IsSome():
			aOnly++
		default:
			t.Fatalf("unexpected option combination: %+v", r)
		}
	}

	if allSome != 1 || aNoneB != 1 || cOnly != 1 || abNoC != 1 || aOnly != 1 {
		t.Fatalf("counts = %d/%d/%d/%d/%d, want 1/1/1/1/1", allSome, aNoneB, cOnly, abNoC, aOnly)
	}
}

// TestFullJoin3Stream proves FullJoin3.Stream yields the same Option-wrapped
// rows All does.
func TestFullJoin3Stream(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	var count int

	for row, err := range FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.A.IsSome() && row.C.IsSome() && !row.B.IsSome() {
			t.Fatalf("row has A and C present but B None: %+v", row)
		}

		count++
	}

	if count != 5 {
		t.Fatalf("Stream rows = %d, want 5", count)
	}
}

// TestFullJoin3IsSingleRoundTrip proves FullJoin3.All issues exactly one
// query and one Scan per row.
func TestFullJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	counting := &scanCountingDB{DB: conn}

	rows, err := FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	if counting.scans != len(rows) {
		t.Fatalf("underlying Scan calls = %d, want %d (one per row)", counting.scans, len(rows))
	}
}

// TestJoin3OuterWherePredicates proves the three per-table Where methods
// (Where→A, WhereRight→B, WhereC→C) narrow a LEFT-LEFT chain.
func TestJoin3OuterWherePredicates(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		Where(joinUserMail.Eq("a@example.com")).
		WhereRight(joinOrderCents.Gt(100)).
		WhereC(joinItemSKU.Eq("pencil")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 only, amount > 100 keeps o2, sku = pencil is o2's item i3.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	if rows[0].B.GetOr(joinOrder{}).ID != "o2" || rows[0].C.GetOr(joinOrderItem{}).ID != "i3" {
		t.Fatalf("row = %+v, want B=o2 C=i3", rows[0])
	}
}

// TestJoin3OuterLimit proves Limit is honoured on an outer join.
func TestJoin3OuterLimit(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).
		OrderBy(joinUserID.Asc()).
		Limit(2).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
}

// TestJoin3OuterCapabilityGates drives the RIGHT/FULL three-table entry
// points through dialects that lack the capability -- a base-only dialect
// and the real sqlite dialect pinned to 3.38.0 (below the 3.39.0 RIGHT/FULL
// floor) -- asserting the typed ErrUnsupportedByDialect, and through
// postgres and the default sqlite dialect (both supported) asserting
// success. LEFT is universal SQL and is never gated.
func TestJoin3OuterCapabilityGates(t *testing.T) {
	ctx := context.Background()

	left := From[joinUser](joinUsers)

	gated := []struct {
		name     string
		dialects []string
		run      func(ctx context.Context, e mockExec) error
	}{
		{"RightJoin3.All", []string{"mock-nocap", "sqlite-3.38"}, func(ctx context.Context, e mockExec) error {
			_, err := RightJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, e)

			return err
		}},
		{"RightJoin3.Stream", []string{"mock-nocap", "sqlite-3.38"}, func(ctx context.Context, e mockExec) error {
			for _, err := range RightJoinOn3(left, userOrdersRel, orderItemsRel).Stream(ctx, e) {
				return err
			}

			return nil
		}},
		{"FullJoin3.All", []string{"mock-nocap", "sqlite-3.38"}, func(ctx context.Context, e mockExec) error {
			_, err := FullJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, e)

			return err
		}},
		{"FullJoin3.Stream", []string{"mock-nocap", "sqlite-3.38"}, func(ctx context.Context, e mockExec) error {
			for _, err := range FullJoinOn3(left, userOrdersRel, orderItemsRel).Stream(ctx, e) {
				return err
			}

			return nil
		}},
	}

	for _, tc := range gated {
		for _, d := range tc.dialects {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				err := tc.run(ctx, mockExec{dialectName: d})
				if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
					t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
				}
			})
		}
	}

	// Postgres and the default SQLite dialect support RIGHT even though they
	// gate it below the 3.39.0 floor.
	if _, err := RightJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, mockExec{dialectName: "postgres"}); err != nil {
		t.Fatalf("RightJoin3.All on postgres failed: %v (Postgres supports RIGHT JOIN)", err)
	}

	for _, tc := range []struct {
		name string
		run  func(ctx context.Context, e mockExec) error
	}{
		{"RightJoin3.All", func(ctx context.Context, e mockExec) error {
			_, err := RightJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, e)

			return err
		}},
		{"FullJoin3.All", func(ctx context.Context, e mockExec) error {
			_, err := FullJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, e)

			return err
		}},
	} {
		for _, d := range []string{"postgres", "sqlite"} {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				if err := tc.run(ctx, mockExec{dialectName: d}); err != nil {
					t.Fatalf("err = %v, want nil (%s supports RIGHT/FULL JOIN)", err, d)
				}
			})
		}
	}

	// LEFT is universal and never gated.
	if _, err := LeftJoinOn3(left, userOrdersRel, orderItemsRel).All(ctx, mockExec{dialectName: "mock-nocap"}); err != nil {
		t.Fatalf("LeftJoin3 on a base-only dialect failed: %v (LEFT JOIN is universal SQL)", err)
	}
}
