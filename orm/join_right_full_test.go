package orm

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// These tests run genuine RIGHT/FULL SQL through the DEFAULT sqlite dialect
// and a real in-memory database: the modernc.org/sqlite driver bundles
// SQLite 3.46.0, and sqlite.New() pins that version, so the dialect's
// version-gated SupportsRightJoin/SupportsFullJoin (≥3.39.0) both report
// true. capability_test.go proves the same entry points are rejected with a
// typed dialect.ErrUnsupportedByDialect when the dialect is pinned below
// 3.39.0.

// newOuterJoinDB seeds the same join_users/join_orders schema as newJoinDB
// but with the extra rows an outer join needs to be observable: a user
// (u2) with NO orders, and an orphan order (o9) whose user_id matches no
// user. It is a fresh in-memory database, so the shared table NAMES do not
// collide with newJoinDB's.
func newOuterJoinDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE join_users (id text, email text)`); err != nil {
		t.Fatalf("create join_users: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_orders (id text, user_id text, amount_cents integer)`); err != nil {
		t.Fatalf("create join_orders: %v", err)
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
		{"o2", "u1", 200},
		{"o3", "u1", 300},
		{"o9", "ghost", 999},
	}

	for _, o := range orders {
		if _, err := conn.Exec(ctx, `INSERT INTO join_orders (id, user_id, amount_cents) VALUES (?, ?, ?)`, o.id, o.userID, o.cents); err != nil {
			t.Fatalf("insert order: %v", err)
		}
	}

	return ctx, conn
}

// TestRightJoin2AllWrapsUnmatchedLeftInNone is THE RIGHT JOIN proof test:
// every order row survives (RIGHT JOIN preserves all of B), u1's three
// orders come back with A = Some(u1), and the orphan order o9 -- whose
// user_id matches no user -- comes back with A = None, never a zero-valued
// joinUser{} that could be mistaken for a real match.
func TestRightJoin2AllWrapsUnmatchedLeftInNone(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	rows, err := RightJoinOn(From[joinUser](joinUsers), userOrdersRel).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 has 3 orders; the orphan o9 adds 1. u2 has no orders, so it is
	// absent from a RIGHT JOIN.
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	var (
		someA  int
		orphan bool
	)

	for _, r := range rows {
		if r.A.IsSome() {
			someA++
		}

		if !r.A.IsSome() && r.B.ID == "o9" {
			orphan = true
		}
	}

	if someA != 3 {
		t.Fatalf("someA = %d, want 3 (u1's orders)", someA)
	}

	if !orphan {
		t.Fatalf("orphan order o9 did not come back with A = None")
	}
}

// TestFullJoin2AllWrapsBothSides is THE FULL JOIN proof test: a full outer
// join keeps BOTH unmatched sides, so u2 (no orders) comes back with
// B = None and the orphan o9 (no user) comes back with A = None -- each
// side's Option wrap is unambiguous.
func TestFullJoin2AllWrapsBothSides(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	rows, err := FullJoinOn(From[joinUser](joinUsers), userOrdersRel).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// 3 matched (Some/Some), 1 orphan (None/Some), 1 orderless user
	// (Some/None).
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	var (
		both, leftOnly, rightOnly int
	)

	for _, r := range rows {
		switch {
		case r.A.IsSome() && r.B.IsSome():
			both++
		case r.A.IsSome() && !r.B.IsSome():
			leftOnly++
		case !r.A.IsSome() && r.B.IsSome():
			rightOnly++
		default:
			t.Fatalf("row with both sides None: %+v", r)
		}
	}

	if both != 3 {
		t.Fatalf("both = %d, want 3", both)
	}

	if leftOnly != 1 {
		t.Fatalf("leftOnly (u2, no orders) = %d, want 1", leftOnly)
	}

	if rightOnly != 1 {
		t.Fatalf("rightOnly (orphan o9) = %d, want 1", rightOnly)
	}
}

// TestRightJoin2Stream proves RightJoin2.Stream yields the same Option-wrapped
// rows All does, including the None (orphan) case.
func TestRightJoin2Stream(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	var none, some int

	for row, err := range RightJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.A.IsSome() {
			some++
		} else {
			none++
		}
	}

	if some != 3 || none != 1 {
		t.Fatalf("Stream some=%d none=%d, want some=3 none=1", some, none)
	}
}

// TestFullJoin2Stream proves FullJoin2.Stream yields the same Option-wrapped
// rows All does.
func TestFullJoin2Stream(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	var both, leftOnly, rightOnly int

	for row, err := range FullJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		switch {
		case row.A.IsSome() && row.B.IsSome():
			both++
		case row.A.IsSome() && !row.B.IsSome():
			leftOnly++
		case !row.A.IsSome() && row.B.IsSome():
			rightOnly++
		}
	}

	if both != 3 || leftOnly != 1 || rightOnly != 1 {
		t.Fatalf("Stream both=%d leftOnly=%d rightOnly=%d, want 3/1/1", both, leftOnly, rightOnly)
	}
}

// TestJoin2OrderByRight proves Join2.OrderByRight orders by a right-table
// column while Join2.OrderBy stays left-scoped -- the ORDER BY analog of
// the Where/WhereRight split.
func TestJoin2OrderByRight(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).
		OrderByRight(joinOrderCents.Desc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	want := []int64{300, 200, 100}
	for i, r := range rows {
		if r.B.AmountCents != want[i] {
			t.Fatalf("row %d amount = %d, want %d (descending)", i, r.B.AmountCents, want[i])
		}
	}
}

// TestLeftJoin2OrderByRight proves LeftJoin2.OrderByRight orders the Some
// rows by a right-table column even when the unmatched row (B = None) is
// present.
func TestLeftJoin2OrderByRight(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).
		OrderByRight(joinOrderCents.Desc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	var amounts []int64
	for _, r := range rows {
		if b, ok := r.B.Get(); ok {
			amounts = append(amounts, b.AmountCents)
		}
	}

	want := []int64{300, 200, 100}
	if len(amounts) != len(want) {
		t.Fatalf("Some amounts = %v, want %v", amounts, want)
	}

	for i := range want {
		if amounts[i] != want[i] {
			t.Fatalf("Some amounts = %v, want %v (descending)", amounts, want)
		}
	}
}

// TestRightJoin2OrderByRight proves right-table ordering works on a RIGHT
// JOIN, where the orphan's 999 amount sorts first in descending order.
func TestRightJoin2OrderByRight(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	rows, err := RightJoinOn(From[joinUser](joinUsers), userOrdersRel).
		OrderByRight(joinOrderCents.Desc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	want := []int64{999, 300, 200, 100}
	for i, r := range rows {
		if r.B.AmountCents != want[i] {
			t.Fatalf("row %d amount = %d, want %d", i, r.B.AmountCents, want[i])
		}
	}
}

// TestFullJoin2OrderByRight proves right-table ordering works on a FULL
// JOIN, where the orderless user's B = None row sorts by NULL.
func TestFullJoin2OrderByRight(t *testing.T) {
	ctx, conn := newOuterJoinDB(t)

	rows, err := FullJoinOn(From[joinUser](joinUsers), userOrdersRel).
		OrderByRight(joinOrderCents.Desc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5", len(rows))
	}

	var amounts []int64
	for _, r := range rows {
		if b, ok := r.B.Get(); ok {
			amounts = append(amounts, b.AmountCents)
		}
	}

	want := []int64{999, 300, 200, 100}
	if len(amounts) != len(want) {
		t.Fatalf("Some amounts = %v, want %v", amounts, want)
	}

	for i := range want {
		if amounts[i] != want[i] {
			t.Fatalf("Some amounts = %v, want %v", amounts, want)
		}
	}
}

// TestJoinOnRightJoinRealSQLite runs the unwrapped JoinOn(..., RightJoin)
// path through the DEFAULT sqlite dialect (bundled 3.46.0) against a real
// in-memory database. newJoinDB's u2 has no orders, so a RIGHT JOIN never
// emits an all-NULL left side here -- every returned row scans cleanly into
// the non-nullable joinUser and matches u1's three orders.
func TestJoinOnRightJoinRealSQLite(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, RightJoin).
		OrderByRight(joinOrderID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (one per order)", len(rows))
	}

	for i, r := range rows {
		if r.A.ID != "u1" || r.A.Email != "a@example.com" || r.B.UserID != "u1" {
			t.Fatalf("rows[%d] = %+v, want A=u1 and B owned by u1", i, r)
		}
	}
}

// TestJoinOnFullJoinRealSQLite runs the unwrapped JoinOn(..., FullJoin) path
// through the DEFAULT sqlite dialect against a real in-memory database. The
// A-side WHERE drops the one all-NULL left row (u2 has no orders), so every
// remaining row has both sides present and scans cleanly.
func TestJoinOnFullJoinRealSQLite(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, FullJoin).
		Where(joinUserMail.Eq("a@example.com")).
		OrderByRight(joinOrderID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (one per order)", len(rows))
	}

	for i, r := range rows {
		if r.A.ID != "u1" || r.B.UserID != "u1" {
			t.Fatalf("rows[%d] = %+v, want A=u1 and B owned by u1", i, r)
		}
	}
}
