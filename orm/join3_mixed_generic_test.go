package orm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// allMixedKinds is the full 4x4 join-kind space MixedJoin3 must cover
// uniformly.
var allMixedKinds = []JoinType{InnerJoin, LeftJoin, RightJoin, FullJoin}

// mixedSig renders one fully-optional row as a compact "A/B/C" signature,
// with "-" for a None side, so a result set can be compared exhaustively
// against a hand-derived expectation.
func mixedSig(r Row3[Option[joinUser], Option[joinOrder], Option[joinOrderItem]]) string {
	sig := func(id string, some bool) string {
		if !some {
			return "-"
		}

		return id
	}

	a, aSome := "", r.A.IsSome()
	if aSome {
		a = r.A.GetOr(joinUser{}).ID
	}

	b, bSome := "", r.B.IsSome()
	if bSome {
		b = r.B.GetOr(joinOrder{}).ID
	}

	c, cSome := "", r.C.IsSome()
	if cSome {
		c = r.C.GetOr(joinOrderItem{}).ID
	}

	return fmt.Sprintf("%s/%s/%s", sig(a, aSome), sig(b, bSome), sig(c, cSome))
}

// mixedSigs materializes the result's signatures, sorted, for an
// order-independent multiset comparison.
func mixedSigs(rows []Row3[Option[joinUser], Option[joinOrder], Option[joinOrderItem]]) []string {
	out := make([]string, 0, len(rows))

	for _, r := range rows {
		out = append(out, mixedSig(r))
	}

	sort.Strings(out)

	return out
}

// mixedAll runs MixedJoinOn3 for one kind pair via All.
func mixedAll(ctx context.Context, exec db.DB, ab, bc JoinType) ([]Row3[Option[joinUser], Option[joinOrder], Option[joinOrderItem]], error) {
	return MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, ab, bc).All(ctx, exec)
}

// mixedComboExpected maps each of the 16 (abKind, bcKind) pairs to the exact
// multiset of "A/B/C" signatures the flat, left-associative chain
// (A abKind B) bcKind C must produce over the newOuterJoin3DB fixture:
//
//	users u1, u2
//	orders o1(u1,100), o5(u1,50), o9(ghost,900) -- orphan o9, orderless u2
//	items i1(o1,pen), i2(o2,apple), i3(o9,zebra) -- orphan i2, unmatched o5
//
// The expectations are hand-derived from the intermediate (A abKind B) row
// set (inner: matched only; left: adds u2/∅; right: adds ∅/o9; full: both),
// then the second hop applied to it with bcKind. They are the load-bearing
// proof that every combination is implemented and that the fully-optional
// scan reports the right side presence.
var mixedComboExpected = map[[2]JoinType][]string{
	{InnerJoin, InnerJoin}: {"u1/o1/i1"},
	{InnerJoin, LeftJoin}:  {"u1/o1/i1", "u1/o5/-"},
	{InnerJoin, RightJoin}: {"u1/o1/i1", "-/-/i2", "-/-/i3"},
	{InnerJoin, FullJoin}:  {"u1/o1/i1", "u1/o5/-", "-/-/i2", "-/-/i3"},
	{LeftJoin, InnerJoin}:  {"u1/o1/i1"},
	{LeftJoin, LeftJoin}:   {"u1/o1/i1", "u1/o5/-", "u2/-/-"},
	{LeftJoin, RightJoin}:  {"u1/o1/i1", "-/-/i2", "-/-/i3"},
	{LeftJoin, FullJoin}:   {"u1/o1/i1", "u1/o5/-", "u2/-/-", "-/-/i2", "-/-/i3"},
	{RightJoin, InnerJoin}: {"u1/o1/i1", "-/o9/i3"},
	{RightJoin, LeftJoin}:  {"u1/o1/i1", "u1/o5/-", "-/o9/i3"},
	{RightJoin, RightJoin}: {"u1/o1/i1", "-/-/i2", "-/o9/i3"},
	{RightJoin, FullJoin}:  {"u1/o1/i1", "-/o9/i3", "u1/o5/-", "-/-/i2"},
	{FullJoin, InnerJoin}:  {"u1/o1/i1", "-/o9/i3"},
	{FullJoin, LeftJoin}:   {"u1/o1/i1", "u1/o5/-", "u2/-/-", "-/o9/i3"},
	{FullJoin, RightJoin}:  {"u1/o1/i1", "-/-/i2", "-/o9/i3"},
	{FullJoin, FullJoin}:   {"u1/o1/i1", "-/o9/i3", "u1/o5/-", "u2/-/-", "-/-/i2"},
}

// TestMixedJoin3AllCombosMatrix drives ALL 16 (abKind, bcKind) pairs against
// real SQLite through the DEFAULT sqlite dialect (bundled 3.46.0, so its
// version-gated RIGHT/FULL support is on), asserting the exact row count and
// Option presence for every chain.
func TestMixedJoin3AllCombosMatrix(t *testing.T) {
	ctx, conn := newOuterJoin3DB(t)

	exec := conn

	for _, ab := range allMixedKinds {
		for _, bc := range allMixedKinds {
			want := mixedComboExpected[[2]JoinType{ab, bc}]

			t.Run(ab.String()+","+bc.String(), func(t *testing.T) {
				rows, err := mixedAll(ctx, exec, ab, bc)
				if err != nil {
					t.Fatalf("All: %v", err)
				}

				if len(rows) != len(want) {
					t.Fatalf("len(rows) = %d, want %d (%v)", len(rows), len(want), want)
				}

				got := mixedSigs(rows)
				wantSorted := append([]string(nil), want...)

				sort.Strings(wantSorted)

				for i := range wantSorted {
					if got[i] != wantSorted[i] {
						t.Fatalf("signatures = %v, want %v", got, want)
					}
				}
			})
		}
	}
}

// TestMixedJoin3WherePredicates proves Where (A), WhereRight (B) and WhereC
// (C) all narrow the generic chain.
func TestMixedJoin3WherePredicates(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, InnerJoin).
		Where(joinUserMail.Eq("a@example.com")).
		WhereRight(joinOrderCents.Gt(100)).
		WhereC(joinItemSKU.Eq("pencil")).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 only, amount > 100 keeps o2 (200) and o3 (300); sku = pencil is o2's
	// item i3.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	if rows[0].B.GetOr(joinOrder{}).ID != "o2" || rows[0].C.GetOr(joinOrderItem{}).ID != "i3" {
		t.Fatalf("row = %+v, want B=o2 C=i3", rows[0])
	}
}

// TestMixedJoin3OrderLimitOffset proves the per-table ordering split and the
// Limit/Offset tail on a generic INNER-then-LEFT chain (supported by base
// SQLite).
func TestMixedJoin3OrderLimitOffset(t *testing.T) {
	ctx, conn := newJoinDB(t)

	base := MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).
		OrderBy(joinUserID.Asc()).
		OrderByRight(joinOrderCents.Asc()).
		OrderByC(joinItemSKU.Asc())

	// newJoinDB: u1's o1 (100) has pen+paper, o2 (200) has pencil, o3 (300)
	// has no items. Ordered by amount then sku => paper, pen, pencil, None.
	rows, err := base.All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantOrder := []struct {
		order string
		sku   string
		none  bool
	}{
		{"o1", "paper", false},
		{"o1", "pen", false},
		{"o2", "pencil", false},
		{"o3", "", true},
	}

	if len(rows) != len(wantOrder) {
		t.Fatalf("len(rows) = %d, want %d", len(rows), len(wantOrder))
	}

	for i, w := range wantOrder {
		if rows[i].B.GetOr(joinOrder{}).ID != w.order {
			t.Fatalf("rows[%d].B = %q, want %q", i, rows[i].B.GetOr(joinOrder{}).ID, w.order)
		}

		if w.none {
			if rows[i].C.IsSome() {
				t.Fatalf("rows[%d].C = Some, want None", i)
			}
		} else if rows[i].C.GetOr(joinOrderItem{}).SKU != w.sku {
			t.Fatalf("rows[%d].C.SKU = %q, want %q", i, rows[i].C.GetOr(joinOrderItem{}).SKU, w.sku)
		}
	}

	// Limit keeps the first two (o1's items only).
	limited, err := base.Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("All limit: %v", err)
	}

	if len(limited) != 2 {
		t.Fatalf("len(limited) = %d, want 2", len(limited))
	}

	for i, r := range limited {
		if r.B.GetOr(joinOrder{}).ID != "o1" {
			t.Fatalf("limited[%d].B = %q, want o1", i, r.B.GetOr(joinOrder{}).ID)
		}
	}

	// Offset skips the first two, leaving pencil then o3's None row.
	offset, err := base.Limit(10).Offset(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("All offset: %v", err)
	}

	if len(offset) != 2 {
		t.Fatalf("len(offset) = %d, want 2", len(offset))
	}

	if offset[0].C.GetOr(joinOrderItem{}).SKU != "pencil" {
		t.Fatalf("offset[0].C.SKU = %q, want pencil", offset[0].C.GetOr(joinOrderItem{}).SKU)
	}

	if offset[1].B.GetOr(joinOrder{}).ID != "o3" || offset[1].C.IsSome() {
		t.Fatalf("offset[1] = %+v, want o3 with C None", offset[1])
	}
}

// TestMixedJoin3Stream proves the generic builder's Stream yields the same
// Option-wrapped rows All does, including a None side.
func TestMixedJoin3Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	for row, err := range MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.B.IsSome() && row.B.GetOr(joinOrder{}).ID == "o3" && row.C.IsSome() {
			t.Fatalf("o3 should have C None, got %+v", row)
		}
	}

	all, err := mixedAll(ctx, conn, InnerJoin, LeftJoin)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	var streamed int

	for range MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).Stream(ctx, conn) {
		streamed++
	}

	if streamed != len(all) {
		t.Fatalf("Stream rows = %d, want %d (same as All)", streamed, len(all))
	}
}

// TestMixedJoin3IsSingleRoundTrip proves a generic chain still issues exactly
// one query, never N+1.
func TestMixedJoin3IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := mixedAll(ctx, counting, InnerJoin, LeftJoin)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("MixedJoin3.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestMixedJoin3IsSingleScanPerRow proves each returned row is populated with
// exactly ONE underlying rows.Scan call, however many sides are optional.
func TestMixedJoin3IsSingleScanPerRow(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &scanCountingDB{DB: conn}

	rows, err := mixedAll(ctx, counting, InnerJoin, LeftJoin)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if counting.scans != len(rows) {
		t.Fatalf("underlying Scan calls = %d, want %d (one per row)", counting.scans, len(rows))
	}
}

// mixedKindSupported reports whether d can render hop kind k, mirroring
// requireJoin: INNER/LEFT are universal, RIGHT/FULL need the matching
// JoinCapabilities boolean.
func mixedKindSupported(d dialect.Dialect, k JoinType) bool {
	if k != RightJoin && k != FullJoin {
		return true
	}

	jd, ok := d.(dialect.JoinCapabilities)
	if !ok {
		return false
	}

	if k == RightJoin {
		return jd.SupportsRightJoin()
	}

	return jd.SupportsFullJoin()
}

// TestMixedJoin3CapabilityGates proves the per-hop gate: a combo is rejected
// with the typed dialect.ErrUnsupportedByDialect iff EITHER hop needs a
// keyword the dialect lacks. A base-only dialect and sqlite pinned to 3.38.0
// (below the 3.39.0 RIGHT/FULL floor) reject every RIGHT/FULL hop (only the
// four INNER/LEFT combos run); the default sqlite dialect (3.46.0) allows
// all 16; postgres allows both.
// All is checked before any SQL is issued, and Stream yields the same typed
// error.
func TestMixedJoin3CapabilityGates(t *testing.T) {
	ctx := t.Context()

	dialects := []string{"mock-nocap", "sqlite-3.38", "sqlite", "postgres"}

	for _, ab := range allMixedKinds {
		for _, bc := range allMixedKinds {
			for _, name := range dialects {
				t.Run(ab.String()+","+bc.String()+"/"+name, func(t *testing.T) {
					e := mockExec{dialectName: name}

					jd, err := dialect.For(name)
					if err != nil {
						t.Fatalf("dialect.For(%q): %v", name, err)
					}

					wantOK := mixedKindSupported(jd, ab) && mixedKindSupported(jd, bc)

					_, err = mixedAll(ctx, e, ab, bc)

					if wantOK {
						if err != nil {
							t.Fatalf("All err = %v, want nil (dialect supports the combo)", err)
						}
					} else if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
						t.Fatalf("All err = %v, want errors.Is(_, ErrUnsupportedByDialect)", err)
					}

					var streamErr error

					for _, err := range MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, ab, bc).Stream(ctx, e) {
						streamErr = err

						break
					}

					if wantOK {
						if streamErr != nil {
							t.Fatalf("Stream err = %v, want nil", streamErr)
						}
					} else if !errors.Is(streamErr, dialect.ErrUnsupportedByDialect) {
						t.Fatalf("Stream err = %v, want errors.Is(_, ErrUnsupportedByDialect)", streamErr)
					}
				})
			}
		}
	}
}

// TestMixedJoin3RendersFlatExactSQL pins the flat, left-associative SQL for a
// LEFT-then-RIGHT and an INNER-then-FULL chain on Postgres, and proves the
// `$N` placeholder numbering is unaffected by the per-hop kinds: the A-side
// WHERE is $1 and the trailing LIMIT is $2.
func TestMixedJoin3RendersFlatExactSQL(t *testing.T) {
	reversed, args, err := MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, LeftJoin, RightJoin).
		Where(joinUserMail.Eq("a@example.com")).
		Limit(2).
		render(postgres.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	wantReversed := `SELECT "join_users"."id", "join_users"."email", "join_orders"."id", "join_orders"."user_id", "join_orders"."amount_cents", "join_order_items"."id", "join_order_items"."order_id", "join_order_items"."sku" FROM "join_users" LEFT JOIN "join_orders" ON "join_users"."id" = "join_orders"."user_id" RIGHT JOIN "join_order_items" ON "join_orders"."id" = "join_order_items"."order_id" WHERE "join_users"."email" = $1 LIMIT $2`

	if reversed != wantReversed {
		t.Fatalf("LEFT-then-RIGHT SQL:\n got %q\nwant %q", reversed, wantReversed)
	}

	if len(args) != 2 || args[0] != "a@example.com" || args[1] != 2 {
		t.Fatalf("args = %#v, want [a@example.com 2]", args)
	}

	full, _, err := MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, FullJoin).
		render(postgres.New())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	wantFullInner := `SELECT "join_users"."id", "join_users"."email", "join_orders"."id", "join_orders"."user_id", "join_orders"."amount_cents", "join_order_items"."id", "join_order_items"."order_id", "join_order_items"."sku" FROM "join_users" INNER JOIN "join_orders" ON "join_users"."id" = "join_orders"."user_id" FULL JOIN "join_order_items" ON "join_orders"."id" = "join_order_items"."order_id"`

	if full != wantFullInner {
		t.Fatalf("INNER-then-FULL SQL:\n got %q\nwant %q", full, wantFullInner)
	}
}
