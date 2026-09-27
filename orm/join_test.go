package orm

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// joinUser/joinOrder are a small fixture pair mirroring what
// schema codegen would generate for a has_many/belongs_to
// relation: two plain entity structs, one orm.Table/orm.Column set per
// entity, and a orm.NewRelation linking them.
type joinUser struct {
	ID    string
	Email string
}

func (u *joinUser) Scan(row Row) error {
	return row.Scan(&u.ID, &u.Email)
}

type joinOrder struct {
	ID          string
	UserID      string
	AmountCents int64
}

func (o *joinOrder) Scan(row Row) error {
	return row.Scan(&o.ID, &o.UserID, &o.AmountCents)
}

// joinOrderItem is the third table of the Join3 fixture: a belongs_to
// child of joinOrder, mirroring the same schema codegen shape.
type joinOrderItem struct {
	ID      string
	OrderID string
	SKU     string
}

func (i *joinOrderItem) Scan(row Row) error {
	return row.Scan(&i.ID, &i.OrderID, &i.SKU)
}

// joinProfile is a third table (belongs_to joinUser) used ONLY by the
// correlated-join tests: it lets a correlated subquery reference a table
// that is NOT one side of the join, so a positive filter can prove the
// marker actually binds the enclosing join's table (a correlated EXISTS
// over a joined table would otherwise be redundant with the join itself).
type joinProfile struct {
	ID     string
	UserID string
	Tier   string
}

func (p *joinProfile) Scan(row Row) error {
	return row.Scan(&p.ID, &p.UserID, &p.Tier)
}

var (
	joinUsers    = NewTable[joinUser]("join_users", []string{"id", "email"})
	joinUserID   = NewColumn[joinUser, string]("join_users", "id")
	joinUserMail = NewColumn[joinUser, string]("join_users", "email")

	joinOrders     = NewTable[joinOrder]("join_orders", []string{"id", "user_id", "amount_cents"})
	joinOrderID    = NewColumn[joinOrder, string]("join_orders", "id")
	joinOrderUser  = NewColumn[joinOrder, string]("join_orders", "user_id")
	joinOrderCents = NewColumn[joinOrder, int64]("join_orders", "amount_cents")

	joinOrderItems  = NewTable[joinOrderItem]("join_order_items", []string{"id", "order_id", "sku"})
	joinItemOrderID = NewColumn[joinOrderItem, string]("join_order_items", "order_id")
	joinItemSKU     = NewColumn[joinOrderItem, string]("join_order_items", "sku")

	joinProfiles    = NewTable[joinProfile]("join_profiles", []string{"id", "user_id", "tier"})
	joinProfileUser = NewColumn[joinProfile, string]("join_profiles", "user_id")

	userOrdersRel = NewRelation[joinUser, joinOrder]("id", "user_id", joinOrders)
	orderItemsRel = NewRelation[joinOrder, joinOrderItem]("id", "order_id", joinOrderItems)
)

// newJoinDB opens an in-memory sqlite database seeded with two users, one
// of which (u2) has NO orders -- the fixture the LEFT JOIN nullability
// test needs.
func newJoinDB(t *testing.T) (context.Context, db.DB) {
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
	}

	for _, o := range orders {
		if _, err := conn.Exec(ctx, `INSERT INTO join_orders (id, user_id, amount_cents) VALUES (?, ?, ?)`, o.id, o.userID, o.cents); err != nil {
			t.Fatalf("insert order: %v", err)
		}
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_order_items (id text, order_id text, sku text)`); err != nil {
		t.Fatalf("create join_order_items: %v", err)
	}

	items := []struct {
		id, orderID, sku string
	}{
		{"i1", "o1", "pen"},
		{"i2", "o1", "paper"},
		{"i3", "o2", "pencil"},
	}

	for _, it := range items {
		if _, err := conn.Exec(ctx, `INSERT INTO join_order_items (id, order_id, sku) VALUES (?, ?, ?)`, it.id, it.orderID, it.sku); err != nil {
			t.Fatalf("insert order item: %v", err)
		}
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE join_profiles (id text, user_id text, tier text)`); err != nil {
		t.Fatalf("create join_profiles: %v", err)
	}

	// Only u2 has a profile, so a correlated "user has a profile" EXISTS is
	// true for u2 and false for u1 -- the direction the correlated tests
	// need to distinguish a bound marker from a dropped one.
	if _, err := conn.Exec(ctx, `INSERT INTO join_profiles (id, user_id, tier) VALUES (?, ?, ?)`, "p1", "u2", "gold"); err != nil {
		t.Fatalf("insert profile: %v", err)
	}

	return ctx, conn
}

// TestJoin2InnerAllScansBothSides proves an INNER JOIN via Join2 returns a
// typed []Row2[A, B] with correct field values from BOTH tables, via a
// real SQLite in-memory database.
func TestJoin2InnerAllScansBothSides(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (u1 has 3 orders, u2 has none)", len(rows))
	}

	for _, r := range rows {
		if r.A.ID != "u1" {
			t.Fatalf("row A.ID = %q, want u1", r.A.ID)
		}

		if r.A.Email != "a@example.com" {
			t.Fatalf("row A.Email = %q, want a@example.com", r.A.Email)
		}

		if r.B.UserID != "u1" {
			t.Fatalf("row B.UserID = %q, want u1", r.B.UserID)
		}
	}

	total := int64(0)
	for _, r := range rows {
		total += r.B.AmountCents
	}

	if total != 600 {
		t.Fatalf("total amount = %d, want 600", total)
	}
}

// TestJoin2WhereAndWhereRight proves Where (left) and WhereRight (right)
// both narrow the joined result.
func TestJoin2WhereAndWhereRight(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).
		Where(joinUserMail.Eq("a@example.com")).
		WhereRight(joinOrderCents.Gt(150)).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (orders o2/o3)", len(rows))
	}

	for _, r := range rows {
		if r.B.AmountCents <= 150 {
			t.Fatalf("row B.AmountCents = %d, want > 150", r.B.AmountCents)
		}
	}
}

// TestLeftJoin2NullabilityDistinguishesNoMatch is THE proof test for the
// LEFT JOIN nullability contract: a parent row (u2) with no matching child comes back with
// Option[joinOrder]{}.IsSome() == false, never a zero-valued joinOrder{}
// that could be mistaken for a real (if all-zero) match.
func TestLeftJoin2NullabilityDistinguishesNoMatch(t *testing.T) {
	ctx, conn := newJoinDB(t)

	rows, err := LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).
		OrderBy(joinUserID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// u1 has 3 orders (3 rows), u2 has none (1 row, right side None).
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	var (
		u1Rows  int
		u2Found bool
	)

	for _, r := range rows {
		switch r.A.ID {
		case "u1":
			u1Rows++

			if !r.B.IsSome() {
				t.Fatalf("u1's order row came back None, want Some")
			}

			b, _ := r.B.Get()
			if b.UserID != "u1" {
				t.Fatalf("u1's order.UserID = %q, want u1", b.UserID)
			}
		case "u2":
			u2Found = true

			if r.B.IsSome() {
				t.Fatalf("u2 has no orders; want B to be None, got Some(%+v)", r.B.GetOr(joinOrder{}))
			}
		default:
			t.Fatalf("unexpected user id %q", r.A.ID)
		}
	}

	if u1Rows != 3 {
		t.Fatalf("u1Rows = %d, want 3", u1Rows)
	}

	if !u2Found {
		t.Fatalf("u2's unmatched row never appeared")
	}
}

// countingDB wraps a db.DB and counts how many Query calls it forwards --
// mirroring orm/app/app_test.go's identically-shaped countingDB, used
// there to prove the old ORM's WithOrders preload is exactly 2 queries.
// Here it proves Join2/LeftJoin2 issue exactly ONE query, no matter how
// many parent/child rows come back -- the single-round-trip guarantee
// promises over a filter-only join + separate preload query.
type countingDB struct {
	db.DB

	queries int
}

func (c *countingDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	c.queries++

	return c.DB.Query(ctx, query, args...) //nolint:wrapcheck // test double, error passes straight through
}

// TestJoin2IsSingleRoundTrip proves Join2.All issues exactly one query for
// N parent rows with M total child rows -- a genuine single-round-trip
// join, never an N+1 anything.
func TestJoin2IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("Join2.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestLeftJoin2IsSingleRoundTrip is TestJoin2IsSingleRoundTrip's LEFT JOIN
// analog.
func TestLeftJoin2IsSingleRoundTrip(t *testing.T) {
	ctx, conn := newJoinDB(t)

	counting := &countingDB{DB: conn}

	rows, err := LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).All(ctx, counting)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	if counting.queries != 1 {
		t.Fatalf("LeftJoin2.All issued %d queries, want exactly 1", counting.queries)
	}
}

// TestJoin2Stream proves Stream yields the same rows All does.
func TestJoin2Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var got []Row2[joinUser, joinOrder]

	for row, err := range JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).OrderBy(joinUserID.Asc()).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		got = append(got, row)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
}

// TestLeftJoin2Stream proves LeftJoin2.Stream yields the same rows All
// does, including the None case.
func TestLeftJoin2Stream(t *testing.T) {
	ctx, conn := newJoinDB(t)

	var noneCount, someCount int

	for row, err := range LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		if row.B.IsSome() {
			someCount++
		} else {
			noneCount++
		}
	}

	if someCount != 3 {
		t.Fatalf("someCount = %d, want 3", someCount)
	}

	if noneCount != 1 {
		t.Fatalf("noneCount = %d, want 1", noneCount)
	}
}

// joinFlavorCase drives one join flavor's full modifier chain plus every
// execution error path, so each flavor's Where/OrderBy/Limit/Offset bodies
// and All/Stream branches are covered without a live database per case.
type joinFlavorCase struct {
	name string
	// chain applies every modifier the flavor supports and renders.
	chain func() (string, error)
	// all runs All against exec, returning the error (nil on success).
	all func(ctx context.Context, exec db.DB) error
	// stream drains Stream against exec, returning the first error met.
	stream func(ctx context.Context, exec db.DB) error
	// dialect is the mockExec dialect All/Stream succeed on.
	dialect string
	// scanRow is one cleanly-scannable row for the iter-error path.
	scanRow []any
	// badA/badB/badC are rows whose A/B/C half fails the entity Scan,
	// covering each scan-error branch. badC is nil for two-table flavors.
	badA []any
	badB []any
	badC []any
}

func drainJoinStream[T any](seq iter.Seq2[T, error]) error {
	for _, err := range seq {
		if err != nil {
			return err
		}
	}

	return nil
}

// breakJoinStream consumes one row then breaks, returning the rows seen.
func breakJoinStream[T any](seq iter.Seq2[T, error]) (int, error) {
	n := 0

	for _, err := range seq {
		if err != nil {
			return n, err
		}

		n++

		break
	}

	return n, nil
}

// Shared scan rows for flavor cases. The two-table flavors all scan the
// same row shape; the three-table flavors share theirs. Package-level
// vars (not literals per case) keep the table compact; runners only read.
var (
	joinScanRow2 = []any{"u1", "a@example.com", "o1", "u1", int64(100)}
	joinBadA2    = []any{nil, "a@example.com", "o1", "u1", int64(100)}
	joinBadB2    = []any{"u1", "a@example.com", nil, "u1", int64(100)}
	joinScanRow3 = []any{"u1", "a@example.com", "o1", "u1", int64(100), "i1", "o1", "s1"}
	joinBadA3    = []any{nil, "a@example.com", "o1", "u1", int64(100), "i1", "o1", "s1"}
	joinBadB3    = []any{"u1", "a@example.com", nil, "u1", int64(100), "i1", "o1", "s1"}
	joinBadC3    = []any{"u1", "a@example.com", "o1", "u1", int64(100), nil, "o1", "s1"}
)

// flavor2 is the two-table fluent surface every Join2 flavor shares.
type flavor2[J any] interface {
	Where(Predicate[joinUser]) J
	WhereRight(Predicate[joinOrder]) J
	OrderBy(...OrderTerm[joinUser]) J
	OrderByRight(...OrderTerm[joinOrder]) J
	Limit(int) J
	render(dialect.Dialect) (string, []any, error)
}

// flavor3 extends flavor2 with the third-table modifiers.
type flavor3[J any] interface {
	flavor2[J]
	WhereC(Predicate[joinOrderItem]) J
	OrderByC(...OrderTerm[joinOrderItem]) J
	Offset(int) J
}

func chainFlavor2[J flavor2[J]](d dialect.Dialect, j J) func() (string, error) {
	return func() (string, error) {
		j = j.Where(joinUserID.Eq("u1")).Where(joinUserMail.Eq("a@example.com")).
			WhereRight(joinOrderUser.Eq("u1")).WhereRight(joinOrderID.Eq("o1")).
			OrderBy(joinUserID.Asc()).OrderByRight(joinOrderID.Asc()).
			Limit(5)
		q, _, err := j.render(d)

		return q, err
	}
}

func chainFlavor3[J flavor3[J]](d dialect.Dialect, j J) func() (string, error) {
	return func() (string, error) {
		j = j.Where(joinUserID.Eq("u1")).Where(joinUserMail.Eq("a@example.com")).
			WhereRight(joinOrderUser.Eq("u1")).WhereRight(joinOrderID.Eq("o1")).
			WhereC(joinItemOrderID.Eq("o1")).WhereC(joinItemSKU.Eq("s1")).
			OrderBy(joinUserID.Asc()).OrderByRight(joinOrderID.Asc()).OrderByC(joinItemSKU.Asc()).
			Limit(5).Offset(2)
		q, _, err := j.render(d)

		return q, err
	}
}

func allFlavor[J interface {
	All(context.Context, db.DB) (R, error)
}, R any](j J) func(context.Context, db.DB) error {
	return func(ctx context.Context, exec db.DB) error {
		_, err := j.All(ctx, exec)

		return err
	}
}

func streamFlavor[J interface {
	Stream(context.Context, db.DB) iter.Seq2[T, error]
}, T any](j J) func(context.Context, db.DB) error {
	return func(ctx context.Context, exec db.DB) error {
		return drainJoinStream(j.Stream(ctx, exec))
	}
}

func joinFlavorCases() []joinFlavorCase {
	return []joinFlavorCase{
		{
			name:    "Join2",
			chain:   chainFlavor2(postgres.New(), JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)),
			all:     allFlavor(JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)),
			stream:  streamFlavor(JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin)),
			dialect: "sqlite",
			scanRow: joinScanRow2,
			badA:    joinBadA2,
			badB:    joinBadB2,
		},
		{
			name:    "LeftJoin2",
			chain:   chainFlavor2(postgres.New(), LeftJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			all:     allFlavor(LeftJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			stream:  streamFlavor(LeftJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			dialect: "sqlite",
			scanRow: joinScanRow2,
			badA:    joinBadA2,
			badB:    joinBadB2,
		},
		{
			name:    "RightJoin2",
			chain:   chainFlavor2(postgres.New(), RightJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			all:     allFlavor(RightJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			stream:  streamFlavor(RightJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			dialect: "postgres",
			scanRow: joinScanRow2,
			badA:    joinBadA2,
			badB:    joinBadB2,
		},
		{
			name:    "FullJoin2",
			chain:   chainFlavor2(postgres.New(), FullJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			all:     allFlavor(FullJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			stream:  streamFlavor(FullJoinOn(From[joinUser](joinUsers), userOrdersRel)),
			dialect: "postgres",
			scanRow: joinScanRow2,
			badA:    joinBadA2,
			badB:    joinBadB2,
		},
		{
			name:    "Join3",
			chain:   chainFlavor3(postgres.New(), JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin)),
			all:     allFlavor(JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin)),
			stream:  streamFlavor(JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin)),
			dialect: "sqlite",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "LeftJoin3",
			chain:   chainFlavor3(postgres.New(), LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			all:     allFlavor(LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			stream:  streamFlavor(LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			dialect: "sqlite",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "RightJoin3",
			chain:   chainFlavor3(postgres.New(), RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			all:     allFlavor(RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			stream:  streamFlavor(RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			dialect: "postgres",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "FullJoin3",
			chain:   chainFlavor3(postgres.New(), FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			all:     allFlavor(FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			stream:  streamFlavor(FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			dialect: "postgres",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "InnerLeftJoin3",
			chain:   chainFlavor3(postgres.New(), InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			all:     allFlavor(InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			stream:  streamFlavor(InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			dialect: "sqlite",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "LeftInnerJoin3",
			chain:   chainFlavor3(postgres.New(), LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			all:     allFlavor(LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			stream:  streamFlavor(LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel)),
			dialect: "sqlite",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
		{
			name:    "MixedJoin3",
			chain:   chainFlavor3(postgres.New(), MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, RightJoin, FullJoin)),
			all:     allFlavor(MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, RightJoin, FullJoin)),
			stream:  streamFlavor(MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, RightJoin, FullJoin)),
			dialect: "sqlite",
			scanRow: joinScanRow3,
			badA:    joinBadA3,
			badB:    joinBadB3,
			badC:    joinBadC3,
		},
	}
}

// TestJoinFlavorChains proves every join flavor's full modifier chain
// renders: each Where/WhereRight/WhereC/OrderBy/OrderByRight/OrderByC/
// Limit/Offset body runs (twice for the AND-branch), and the chained
// statement carries all of them.
func TestJoinFlavorChains(t *testing.T) {
	for _, tc := range joinFlavorCases() {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.chain()
			if err != nil {
				t.Fatalf("render: %v", err)
			}

			for _, frag := range []string{"ORDER BY", "LIMIT"} {
				if !strings.Contains(q, frag) {
					t.Fatalf("query %q missing %q", q, frag)
				}
			}
		})
	}
}

// TestJoinFlavorAllStreamHappyPaths runs every flavor's All and Stream
// against an empty mock result set, covering the no-rows loop exits.
func TestJoinFlavorAllStreamHappyPaths(t *testing.T) {
	ctx := t.Context()

	for _, tc := range joinFlavorCases() {
		t.Run(tc.name, func(t *testing.T) {
			e := mockExec{dialectName: tc.dialect}

			if err := tc.all(ctx, e); err != nil {
				t.Fatalf("All: %v", err)
			}

			if err := tc.stream(ctx, e); err != nil {
				t.Fatalf("Stream: %v", err)
			}
		})
	}
}

// TestJoinFlavorAllResolveErrors proves every flavor's All fails closed
// when the dialect cannot be resolved.
func TestJoinFlavorAllResolveErrors(t *testing.T) {
	ctx := t.Context()

	for _, tc := range joinFlavorCases() {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.all(ctx, fakeDB{}); err == nil {
				t.Fatalf("All on an unresolvable dialect succeeded, want an error")
			}

			if err := tc.stream(ctx, fakeDB{}); err == nil {
				t.Fatalf("Stream on an unresolvable dialect succeeded, want an error")
			}
		})
	}
}

// TestJoinFlavorExecutionErrorPaths drives query, scan, iteration and
// close failures through every flavor's All.
func TestJoinFlavorExecutionErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	for _, tc := range joinFlavorCases() {
		t.Run(tc.name+"/query", func(t *testing.T) {
			s := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: &stubRows{}, queryErr: boom}

			if err := tc.all(ctx, s); !errors.Is(err, boom) {
				t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
			}
		})

		t.Run(tc.name+"/scan", func(t *testing.T) {
			s := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: &stubRows{values: [][]any{tc.scanRow}, scanErr: boom}}

			if err := tc.all(ctx, s); !errors.Is(err, boom) {
				t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
			}
		})

		t.Run(tc.name+"/iter", func(t *testing.T) {
			s := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: &stubRows{values: [][]any{tc.scanRow}, iterErr: boom}}

			if err := tc.all(ctx, s); !errors.Is(err, boom) {
				t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
			}
		})

		t.Run(tc.name+"/close", func(t *testing.T) {
			s := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: &stubRows{closeErr: boom}}

			if err := tc.all(ctx, s); !errors.Is(err, boom) {
				t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
			}
		})

		for _, bad := range []struct {
			name string
			row  []any
		}{
			{"badA", tc.badA},
			{"badB", tc.badB},
			{"badC", tc.badC},
		} {
			if bad.row == nil {
				continue
			}

			t.Run(tc.name+"/"+bad.name, func(t *testing.T) {
				s := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: &stubRows{values: [][]any{bad.row}}}

				if err := tc.all(ctx, s); err == nil {
					t.Fatalf("All with %s succeeded, want a scan error", bad.name)
				}
			})
		}

		t.Run(tc.name+"/render", func(t *testing.T) {
			multi := From(widgetOrders)

			var err error

			switch tc.name {
			case "Join2":
				_, err = JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "LeftJoin2":
				_, err = LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "RightJoin2":
				_, err = RightJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "postgres"})
			case "FullJoin2":
				_, err = FullJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "postgres"})
			case "Join3":
				_, err = JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "LeftJoin3":
				_, err = LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "RightJoin3":
				_, err = RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "postgres"})
			case "FullJoin3":
				_, err = FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "postgres"})
			case "InnerLeftJoin3":
				_, err = InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "LeftInnerJoin3":
				_, err = LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			case "MixedJoin3":
				_, err = MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).Where(joinUserID.InSub(multi)).All(ctx, mockExec{dialectName: "sqlite"})
			}

			if err == nil {
				t.Fatalf("All with a multi-column IN subquery succeeded, want a render error")
			}
		})

		t.Run(tc.name+"/stream-errors", func(t *testing.T) {
			newStub := func(rows *stubRows) *ormStubDB {
				return &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: rows}
			}

			queryErr := newStub(&stubRows{})
			queryErr.queryErr = boom

			if err := tc.stream(ctx, queryErr); !errors.Is(err, boom) {
				t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
			}

			scanErr := newStub(&stubRows{values: [][]any{tc.scanRow}, scanErr: boom})

			if err := tc.stream(ctx, scanErr); !errors.Is(err, boom) {
				t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
			}

			iterErr := newStub(&stubRows{values: [][]any{tc.scanRow}, iterErr: boom})

			if err := tc.stream(ctx, iterErr); !errors.Is(err, boom) {
				t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
			}

			closeErr := newStub(&stubRows{closeErr: boom})

			if err := tc.stream(ctx, closeErr); !errors.Is(err, boom) {
				t.Fatalf("Stream err = %v, want errors.Is(err, boom)", err)
			}
		})

		t.Run(tc.name+"/stream-render", func(t *testing.T) {
			multi := From(widgetOrders)

			var err error

			switch tc.name {
			case "Join2":
				err = drainJoinStream(JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "LeftJoin2":
				err = drainJoinStream(LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "RightJoin2":
				err = drainJoinStream(RightJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "postgres"}))
			case "FullJoin2":
				err = drainJoinStream(FullJoinOn(From[joinUser](joinUsers), userOrdersRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "postgres"}))
			case "Join3":
				err = drainJoinStream(JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "LeftJoin3":
				err = drainJoinStream(LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "RightJoin3":
				err = drainJoinStream(RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "postgres"}))
			case "FullJoin3":
				err = drainJoinStream(FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "postgres"}))
			case "InnerLeftJoin3":
				err = drainJoinStream(InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "LeftInnerJoin3":
				err = drainJoinStream(LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			case "MixedJoin3":
				err = drainJoinStream(MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).Where(joinUserID.InSub(multi)).Stream(ctx, mockExec{dialectName: "sqlite"}))
			}

			if err == nil {
				t.Fatal("Stream with a multi-column IN subquery succeeded, want a render error")
			}
		})

		t.Run(tc.name+"/stream-break", func(t *testing.T) {
			rows := &stubRows{values: [][]any{tc.scanRow, tc.scanRow}}
			stub := &ormStubDB{mockExec: mockExec{dialectName: tc.dialect}, rows: rows}

			var err error

			var n int

			switch tc.name {
			case "Join2":
				n, err = breakJoinStream(JoinOn(From[joinUser](joinUsers), userOrdersRel, InnerJoin).Stream(ctx, stub))
			case "LeftJoin2":
				n, err = breakJoinStream(LeftJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, stub))
			case "RightJoin2":
				n, err = breakJoinStream(RightJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, stub))
			case "FullJoin2":
				n, err = breakJoinStream(FullJoinOn(From[joinUser](joinUsers), userOrdersRel).Stream(ctx, stub))
			case "Join3":
				n, err = breakJoinStream(JoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin).Stream(ctx, stub))
			case "LeftJoin3":
				n, err = breakJoinStream(LeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, stub))
			case "RightJoin3":
				n, err = breakJoinStream(RightJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, stub))
			case "FullJoin3":
				n, err = breakJoinStream(FullJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, stub))
			case "InnerLeftJoin3":
				n, err = breakJoinStream(InnerLeftJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, stub))
			case "LeftInnerJoin3":
				n, err = breakJoinStream(LeftInnerJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel).Stream(ctx, stub))
			case "MixedJoin3":
				n, err = breakJoinStream(MixedJoinOn3(From[joinUser](joinUsers), userOrdersRel, orderItemsRel, InnerJoin, LeftJoin).Stream(ctx, stub))
			}

			if err != nil {
				t.Fatalf("Stream: %v", err)
			}

			if n != 1 {
				t.Fatalf("broke after %d rows, want exactly 1", n)
			}
		})
	}
}

// TestJoinTypeString pins every JoinType keyword plus the out-of-range
// INNER fallback used in error messages.
func TestJoinTypeString(t *testing.T) {
	for _, tc := range []struct {
		jt   JoinType
		want string
	}{
		{InnerJoin, "INNER JOIN"},
		{LeftJoin, "LEFT JOIN"},
		{RightJoin, "RIGHT JOIN"},
		{FullJoin, "FULL JOIN"},
		{JoinType(99), "INNER JOIN"},
	} {
		if got := tc.jt.String(); got != tc.want {
			t.Fatalf("JoinType(%d).String() = %q, want %q", int(tc.jt), got, tc.want)
		}
	}
}

// TestAssignAnyBranches pins assignAny's destination dispatch: the
// Option scanner path, every concrete pointer type's ok path, every
// conversion failure, and the closed-set rejection.
func TestAssignAnyBranches(t *testing.T) {
	var (
		s   string
		b   []byte
		i64 int64
		i32 int32
		f64 float64
		f32 float32
		bl  bool
		v   any
		o   Option[string]
	)

	ok := []struct {
		name string
		dest any
		src  any
	}{
		{"any", &v, "x"},
		{"string", &s, "x"},
		{"bytes", &b, []byte("x")},
		{"int64", &i64, int64(7)},
		{"int32", &i32, int64(7)},
		{"float64", &f64, 1.5},
		{"float32", &f32, 1.5},
		{"bool", &bl, true},
		{"option value", &o, "x"},
		{"option null", &o, nil},
	}

	for _, tc := range ok {
		t.Run("ok/"+tc.name, func(t *testing.T) {
			if err := assignAny(tc.dest, tc.src); err != nil {
				t.Fatalf("assignAny(%T, %#v) = %v, want nil", tc.dest, tc.src, err)
			}
		})
	}

	bad := struct{}{}

	errCases := []struct {
		name string
		dest any
	}{
		{"string", &s},
		{"bytes", &b},
		{"int64", &i64},
		{"int32", &i32},
		{"float64", &f64},
		{"float32", &f32},
		{"bool", &bl},
		{"closed", new(struct{})},
	}

	for _, tc := range errCases {
		t.Run("err/"+tc.name, func(t *testing.T) {
			if err := assignAny(tc.dest, bad); err == nil {
				t.Fatalf("assignAny(%T, struct) succeeded, want an error", tc.dest)
			}
		})
	}
}

// TestRowFeedExhausted proves a row feed shorter than the entity's Scan
// reads fails closed instead of panicking on a short row.
func TestRowFeedExhausted(t *testing.T) {
	var a, b string

	if err := (&rowFeed{vals: []any{"x"}}).Scan(&a, &b); err == nil {
		t.Fatal("short rowFeed.Scan succeeded, want a feed-exhausted error")
	}
}
