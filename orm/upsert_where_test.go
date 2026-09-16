package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// partialWidget is a dedicated fixture for the partial-upsert-WHERE tests:
// four plain columns, one of which (active) backs a partial unique index, so
// the ON CONFLICT target WHERE can name the index predicate.
type partialWidget struct {
	ID       string
	Name     string
	Quantity int64
	Active   int64
}

func (p *partialWidget) Scan(row Row) error {
	return row.Scan(&p.ID, &p.Name, &p.Quantity, &p.Active)
}

var (
	partialWidgets = NewTable[partialWidget]("partial_widgets", []string{"id", "name", "quantity", "active"})
	partialSrc     = NewTable[partialWidget]("partial_src", []string{"id", "name", "quantity", "active"})
	pwID           = NewColumn[partialWidget, string]("partial_widgets", "id")
	pwName         = NewColumn[partialWidget, string]("partial_widgets", "name")
	pwQty          = NewColumn[partialWidget, int64]("partial_widgets", "quantity")
	pwActive       = NewColumn[partialWidget, int64]("partial_widgets", "active")
)

// capturingExec is a db.DB that reports a caller-chosen dialect and records
// the last statement (text + args) handed to Exec or Query, so a
// non-SQLite-render test can pin the exact Postgres `$N` numbering through
// the public builder API.
type capturingExec struct {
	mockExec

	query string
	args  []any
}

func (c *capturingExec) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	c.query = query
	c.args = args

	return emptyRows{}, nil
}

func (c *capturingExec) Exec(_ context.Context, query string, args ...any) (int64, error) {
	c.query = query
	c.args = args

	return 0, nil
}

// partialTargetWhere returns the conflict-target predicate naming the
// partial index partial_widgets_id_active (WHERE active = 1). SQLite (and
// Postgres) require the ON CONFLICT target predicate to match the index
// definition literally at prepare time -- a bound parameter cannot be proven
// equivalent -- so the literal predicate is expressed through the audited
// UnsafeRaw escape hatch rather than a parameter-binding Column.Eq.
func partialTargetWhere() Predicate[partialWidget] {
	//lint:allow-unsafesql partial-index conflict targets must match the index definition literally; a bound parameter cannot be proven equivalent at prepare time
	return UnsafeRaw[partialWidget]("active = 1")
}

// newPartialUpsertDB opens an in-memory sqlite database with a partial unique
// index on id (WHERE active = 1), seeded with one active row, plus an empty
// partial_src table for the INSERT ... SELECT round-trips.
func newPartialUpsertDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	for _, stmt := range []string{
		`CREATE TABLE partial_widgets (id text, name text, quantity integer, active integer)`,
		`CREATE UNIQUE INDEX partial_widgets_id_active ON partial_widgets(id) WHERE active = 1`,
		`CREATE TABLE partial_src (id text, name text, quantity integer, active integer)`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	if _, err := conn.Exec(ctx, `INSERT INTO partial_widgets (id, name, quantity, active) VALUES (?, ?, ?, ?)`, "w1", "Alpha", int64(10), int64(1)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return ctx, conn
}

// partialByID groups the partial_widgets rows by id (a partial index permits
// several rows for one id when their active values differ).
func partialByID(ctx context.Context, t *testing.T, conn db.DB) map[string][]*partialWidget {
	t.Helper()

	rows, err := From(partialWidgets).All(ctx, conn)
	if err != nil {
		t.Fatalf("partial All: %v", err)
	}

	out := make(map[string][]*partialWidget, len(rows))
	for _, r := range rows {
		out[r.ID] = append(out[r.ID], r)
	}

	return out
}

// TestUpsertWhereCapabilityTruthTable pins the partial-upsert-WHERE
// capability matrix: Postgres and SQLite support both the conflict-target
// WHERE and the conditional-update WHERE.
func TestUpsertWhereCapabilityTruthTable(t *testing.T) {
	var (
		_ dialect.ConflictWhereDialect = postgres.New()
		_ dialect.ConflictWhereDialect = sqlite.New()
	)

	p := postgres.New()
	if !p.SupportsConflictTargetWhere() || !p.SupportsConflictUpdateWhere() {
		t.Fatalf("postgres = (%v, %v), want (true, true)", p.SupportsConflictTargetWhere(), p.SupportsConflictUpdateWhere())
	}

	s := sqlite.New()
	if !s.SupportsConflictTargetWhere() || !s.SupportsConflictUpdateWhere() {
		t.Fatalf("sqlite = (%v, %v), want (true, true)", s.SupportsConflictTargetWhere(), s.SupportsConflictUpdateWhere())
	}
}

// TestUpsertWhereCapabilityGate proves a requested partial-upsert WHERE is
// never silently dropped: on a base-only dialect (no ConflictWhereDialect)
// it fails with a typed dialect.ErrUnsupportedByDialect, for
// both the target predicate and the update predicate.
func TestUpsertWhereCapabilityGate(t *testing.T) {
	ctx := context.Background()

	targeted := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Alpha"), Set(pwQty, int64(10)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(pwActive.Eq(int64(1))).
		DoNothing()

	conditional := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Alpha"), Set(pwQty, int64(10)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		DoUpdate(Set(pwName, "Beta")).
		Where(pwQty.Lt(int64(100)))

	for _, tc := range []struct {
		name string
		up   Insert[partialWidget]
	}{
		{"target-where", targeted},
		{"update-where", conditional},
	} {
		for _, d := range []string{"mock-nocap"} {
			t.Run(tc.name+"/"+d, func(t *testing.T) {
				err := tc.up.Exec(ctx, mockExec{dialectName: d})
				if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
					t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
				}
			})
		}
	}
}

// TestUpsertTargetWhereDoNothing proves the partial-index target predicate at
// the SQLite round-trip level: an insert whose row matches the partial
// index predicate conflicts and is ignored, while a row whose active value
// fails the predicate does not hit the partial index and inserts alongside.
func TestUpsertTargetWhereDoNothing(t *testing.T) {
	ctx, conn := newPartialUpsertDB(t)

	// active = 1 matches the partial index, so the conflicting row is ignored.
	if err := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "ShouldNotChange"), Set(pwQty, int64(99)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(partialTargetWhere()).
		DoNothing().
		Exec(ctx, conn); err != nil {
		t.Fatalf("target-where DO NOTHING Exec: %v", err)
	}

	got := partialByID(ctx, t, conn)
	if len(got["w1"]) != 1 || got["w1"][0].Name != "Alpha" || got["w1"][0].Quantity != 10 {
		t.Fatalf("after matching DO NOTHING: w1 rows = %+v, want the untouched Alpha/10", got["w1"])
	}

	// active = 0 fails the partial-index predicate, so no conflict: the row
	// inserts and the partial index now holds two w1 rows (one per active
	// value).
	if err := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Inactive"), Set(pwQty, int64(5)), Set(pwActive, int64(0))).
		OnConflict(pwID.Col()).
		Where(partialTargetWhere()).
		DoNothing().
		Exec(ctx, conn); err != nil {
		t.Fatalf("non-matching DO NOTHING Exec: %v", err)
	}

	got = partialByID(ctx, t, conn)
	if len(got["w1"]) != 2 {
		t.Fatalf("after non-matching insert: w1 rows = %+v, want 2 (partial index excluded active=0)", got["w1"])
	}
}

// TestUpsertUpdateWhere proves the conditional DO UPDATE: the SET applies
// only while the update predicate holds and leaves the row unchanged
// otherwise.
func TestUpsertUpdateWhere(t *testing.T) {
	ctx, conn := newPartialUpsertDB(t)

	// Predicate false (existing quantity 10 is not > 1000): row untouched.
	if err := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Renamed"), Set(pwQty, int64(99)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(partialTargetWhere()).
		DoUpdate(Set(pwName, "Renamed"), Set(pwQty, int64(99))).
		Where(pwQty.Gt(int64(1000))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("conditional DO UPDATE (false) Exec: %v", err)
	}

	got := partialByID(ctx, t, conn)
	if len(got["w1"]) != 1 || got["w1"][0].Name != "Alpha" || got["w1"][0].Quantity != 10 {
		t.Fatalf("after false predicate: w1 = %+v, want untouched Alpha/10", got["w1"])
	}

	// Predicate true (existing quantity 10 < 1000): row updated.
	if err := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Renamed"), Set(pwQty, int64(99)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(partialTargetWhere()).
		DoUpdate(Set(pwName, "Renamed"), Set(pwQty, int64(99))).
		Where(pwQty.Lt(int64(1000))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("conditional DO UPDATE (true) Exec: %v", err)
	}

	got = partialByID(ctx, t, conn)
	if len(got["w1"]) != 1 || got["w1"][0].Name != "Renamed" || got["w1"][0].Quantity != 99 {
		t.Fatalf("after true predicate: w1 = %+v, want Renamed/99", got["w1"])
	}
}

// TestUpsertWhereInsertSelect proves both predicates compose with the
// INSERT ... SELECT source at the SQLite round-trip level: the conflicting
// source row is conditionally updated, the new one inserted.
func TestUpsertWhereInsertSelect(t *testing.T) {
	ctx, conn := newPartialUpsertDB(t)

	for _, r := range []struct {
		id, name string
		qty      int64
	}{
		{"w1", "SrcAlpha", 111},
		{"w2", "SrcBeta", 222},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO partial_src (id, name, quantity, active) VALUES (?, ?, ?, ?)`, r.id, r.name, r.qty, int64(1)); err != nil {
			t.Fatalf("seed src: %v", err)
		}
	}

	if err := InsertInto(partialWidgets).
		OnConflict(pwID.Col()).
		Where(partialTargetWhere()).
		DoUpdate(Set(pwName, "Merged")).
		Where(pwQty.Lt(int64(50))).
		Select(From(partialSrc).Where(pwQty.Gt(int64(100)))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("INSERT ... SELECT partial upsert Exec: %v", err)
	}

	got := partialByID(ctx, t, conn)

	// w1 conflicted; existing quantity 10 < 50 so the update ran.
	if len(got["w1"]) != 1 || got["w1"][0].Name != "Merged" || got["w1"][0].Quantity != 10 {
		t.Fatalf("w1 after select upsert = %+v, want Merged with pre-existing quantity 10", got["w1"])
	}

	// w2 was new: inserted unchanged.
	if len(got["w2"]) != 1 || got["w2"][0].Name != "SrcBeta" || got["w2"][0].Quantity != 222 {
		t.Fatalf("w2 after select upsert = %+v, want inserted SrcBeta/222", got["w2"])
	}
}

// TestUpsertWherePostgresPlaceholders pins the exact `$N` numbering through
// the public builder API for a VALUES upsert carrying both predicates: row
// values, then target predicate, then SET, then update predicate.
func TestUpsertWherePostgresPlaceholders(t *testing.T) {
	captured := &capturingExec{mockExec: mockExec{dialectName: "postgres"}}

	err := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Alpha"), Set(pwQty, int64(10)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(pwActive.Eq(int64(1))).
		DoUpdate(Set(pwName, "Beta")).
		Where(pwQty.Lt(int64(1000))).
		Exec(context.Background(), captured)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	want := `INSERT INTO "partial_widgets" ("id", "name", "quantity", "active") VALUES ($1, $2, $3, $4) ON CONFLICT ("id") WHERE "active" = $5 DO UPDATE SET "name" = $6 WHERE "quantity" < $7`
	if captured.query != want {
		t.Fatalf("query = %q, want %q", captured.query, want)
	}
}

// TestUpsertWhereInsertSelectPostgresPlaceholders pins the INSERT ... SELECT
// numbering through the public builder API: the SELECT's args come first,
// then the target predicate, then the SET, then the update predicate.
func TestUpsertWhereInsertSelectPostgresPlaceholders(t *testing.T) {
	captured := &capturingExec{mockExec: mockExec{dialectName: "postgres"}}

	err := InsertInto(partialWidgets).
		OnConflict(pwID.Col()).
		Where(pwActive.Eq(int64(1))).
		DoUpdate(Set(pwName, "Merged")).
		Where(pwQty.Lt(int64(1000))).
		Select(From(partialSrc).Where(pwQty.Gt(int64(100)))).
		Exec(context.Background(), captured)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// The SELECT binds $1 (quantity > 100); target predicate $2, SET name $3,
	// update predicate $4.
	want := `INSERT INTO "partial_widgets" ("id", "name", "quantity", "active") SELECT "id", "name", "quantity", "active" FROM "partial_src" WHERE "quantity" > $1 ON CONFLICT ("id") WHERE "active" = $2 DO UPDATE SET "name" = $3 WHERE "quantity" < $4`
	if captured.query != want {
		t.Fatalf("query = %q, want %q", captured.query, want)
	}
}

// TestUpsertWhereCopyOnWrite proves the Where methods keep the copy-on-write
// discipline: branching a base upsert and attaching different predicates to
// the two branches never leaks one branch's predicate into the other.
func TestUpsertWhereCopyOnWrite(t *testing.T) {
	base := InsertInto(partialWidgets).
		Values(Set(pwID, "w1"), Set(pwName, "Alpha"), Set(pwQty, int64(10)), Set(pwActive, int64(1))).
		OnConflict(pwID.Col()).
		Where(pwActive.Eq(int64(1)))

	a := base.DoUpdate(Set(pwName, "A"))
	b := base.DoUpdate(Set(pwName, "B"))

	aWhere := a.Where(pwQty.Lt(int64(1)))
	bWhere := b.Where(pwQty.Gt(int64(1)))

	if aWhere.conflict.updateWhere.Kind == NNone || bWhere.conflict.updateWhere.Kind == NNone {
		t.Fatalf("branches lost their update predicate: a=%d b=%d", aWhere.conflict.updateWhere.Kind, bWhere.conflict.updateWhere.Kind)
	}

	if aWhere.conflict.updateWhere.Op == bWhere.conflict.updateWhere.Op {
		t.Fatalf("branches share an update predicate (op %d): want independent Lt/Gt", aWhere.conflict.updateWhere.Op)
	}

	// The target predicate must have travelled into both finished inserts.
	if aWhere.conflict.targetWhere.Kind == NNone || bWhere.conflict.targetWhere.Kind == NNone {
		t.Fatalf("target predicate lost: a=%d b=%d", aWhere.conflict.targetWhere.Kind, bWhere.conflict.targetWhere.Kind)
	}
}
