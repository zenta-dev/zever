package orm

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// joinOrderNote is a nullable column on the join_orders fixture used by the
// Join2 NULLS-ordering test. joinOrders deliberately does NOT project it
// (the entity Scan still reads three columns); ORDER BY on a non-projected
// column is valid SQL and proves the modifier reaches the right side's
// ORDER BY list.
var joinOrderNote = NewNullableColumn[joinOrder, string]("join_orders", "note")

// newNullsJoinDB opens an in-memory sqlite database with one user and three
// orders whose note is NULL/“"a"“/“"c"“ -- the fixture the Join2 NULLS
// ordering test sorts.
func newNullsJoinDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE join_users (id text, email text)`); err != nil {
		t.Fatalf("create join_users: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_orders (id text, user_id text, amount_cents integer, note text)`); err != nil {
		t.Fatalf("create join_orders: %v", err)
	}

	if _, err := conn.Exec(ctx, `INSERT INTO join_users (id, email) VALUES (?, ?)`, "u1", "a@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	orders := []struct {
		id, userID string
		cents      int64
		note       any
	}{
		{"o1", "u1", 100, "a"},
		{"o2", "u1", 200, nil},
		{"o3", "u1", 300, "c"},
	}

	for _, o := range orders {
		if _, err := conn.Exec(ctx, `INSERT INTO join_orders (id, user_id, amount_cents, note) VALUES (?, ?, ?, ?)`, o.id, o.userID, o.cents, o.note); err != nil {
			t.Fatalf("insert order: %v", err)
		}
	}

	return ctx, conn
}

// TestNullsModifiersCopyOnWrite proves NullsFirst/NullsLast return modified
// copies and leave the receiver (and its direction) untouched.
func TestNullsModifiersCopyOnWrite(t *testing.T) {
	base := widgetBio.Asc()

	first := base.NullsFirst()
	last := base.NullsLast()

	if base.Nulls != NullsDefault {
		t.Fatalf("base.Nulls = %v, want NullsDefault (receiver mutated)", base.Nulls)
	}

	if first.Nulls != NullsFirst || first.Desc {
		t.Fatalf("first = %+v, want ascending NULLS FIRST", first)
	}

	if last.Nulls != NullsLast || last.Desc {
		t.Fatalf("last = %+v, want ascending NULLS LAST", last)
	}

	descFirst := widgetBio.Desc().NullsFirst()
	if !descFirst.Desc || descFirst.Nulls != NullsFirst {
		t.Fatalf("descFirst = %+v, want descending NULLS FIRST", descFirst)
	}
}

// TestNullsOrderSQLiteOrdersNulls exercises the modifier end to end on a
// real SQLite database: DESC NULLS FIRST pulls the NULL bio to the FRONT
// (SQLite's DESC default is NULLS LAST), and ASC NULLS LAST pushes it to
// the BACK (SQLite's ASC default is NULLS FIRST) -- so each case proves the
// modifier, not the dialect default.
func TestNullsOrderSQLiteOrdersNulls(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	tests := []struct {
		name string
		term OrderTerm[widget]
		want []string
	}{
		{"desc-nulls-first", widgetBio.Desc().NullsFirst(), []string{"w2", "w3", "w1"}},
		{"asc-nulls-last", widgetBio.Asc().NullsLast(), []string{"w1", "w3", "w2"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := From(widgets).OrderBy(tc.term).All(ctx, conn)
			if err != nil {
				t.Fatalf("All: %v", err)
			}

			got := make([]string, len(rows))
			for i, r := range rows {
				got[i] = r.ID
			}

			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestNullsOrderJoin2SQLite proves the modifier survives a Join2 ORDER BY
// (the right table's terms flow through render's qualifyOrderTerms copy).
func TestNullsOrderJoin2SQLite(t *testing.T) {
	ctx, conn := newNullsJoinDB(t)

	tests := []struct {
		name string
		term OrderTerm[joinOrder]
		want []string
	}{
		{"asc-nulls-last", joinOrderNote.Asc().NullsLast(), []string{"o1", "o3", "o2"}},
		{"desc-nulls-first", joinOrderNote.Desc().NullsFirst(), []string{"o2", "o3", "o1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).
				OrderByRight(tc.term).
				All(ctx, conn)
			if err != nil {
				t.Fatalf("All: %v", err)
			}

			got := make([]string, len(rows))
			for i, r := range rows {
				got[i] = r.B.ID
			}

			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestNullsOrderingCaseEmulation documents and exercises the portable
// workaround for engines without the NULLS keyword: order by a CASE that
// ranks NULLs, then by the column. Running it on real SQLite proves the
// emitted SQL is valid standard SQL and orders NULLs last as intended.
func TestNullsOrderingCaseEmulation(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	emulation := Case[widget, int]().When(widgetBio.IsNull(), 1).Else(0).Asc()

	rows, err := From(widgets).OrderBy(emulation, widgetBio.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.ID
	}

	want := []string{"w1", "w3", "w2"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v (CASE emulation pushes NULL last)", got, want)
		}
	}
}
