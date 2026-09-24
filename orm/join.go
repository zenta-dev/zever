package orm

import (
	"context"
	"fmt"
	"iter"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// Relation[Parent, Child] design decision, and Join2's type-parameter
// shape (documented per the Phase 3 task brief):
//
// Query[T, PT] (orm/query.go) already settled the "Scan has a pointer
// receiver" problem with two type parameters: T is the plain entity type,
// PT is constrained to *T plus Scanner. Join2 needs the SAME treatment on
// BOTH sides of the join -- there is no way around it, since All/Stream
// must construct a fresh A and a fresh B per row and scan into each
// through its own pointer-receiver Scan method. The only fix that follows
// Phase 1's own precedent (rather than inventing a new one) is FOUR type
// parameters: Join2[A, PA, B, PB], with PA ptrScanner[A] and PB
// ptrScanner[B] exactly mirroring Query[T, PT]'s PT ptrScanner[T].
//
// This reads more heavily at the type-parameter-list declaration site than
// simplified Join2[A, B] sketch, but every call site stays as
// terse as Query's: Go's constraint type inference resolves PA from A (via
// ptrScanner[A]'s *A core type) and PB from B the same way constraint
// inference already lets `orm.From(Users)` infer PT without the caller
// spelling it out -- so `orm.JoinOn(orm.From(Users), rel, orm.InnerJoin)`
// compiles with no explicit type arguments at all.

// Relation is a typed, single-column equi-join descriptor from Parent to
// Child: "Parent.parentCol = Child.childCol". Its fields are unexported --
// the only way to construct a working Relation is NewRelation, which
// schema codegen-generated code is expected to call with
// schema-derived column names and a real Table[Child] -- the same
// "codegen only, never a caller string" contract Column/Table already
// follow .
//
// Only single-column equi-joins are supported (matching 's
// stated Phase 3 scope); a multi-column join key is out of scope.
type Relation[Parent any, Child any] struct {
	parentCol  string
	childCol   string
	childTable Table[Child]
}

// NewRelation builds a Relation bound to parentCol/childCol/childTable.
// It exists for schema codegen-generated code to call.
func NewRelation[Parent any, Child any](parentCol, childCol string, childTable Table[Child]) Relation[Parent, Child] {
	return Relation[Parent, Child]{parentCol: parentCol, childCol: childCol, childTable: childTable}
}

// Row2 is the typed composite result of a two-table join: A's and B's
// scanned values side by side in one struct, populated by a single
// rows.Scan call (see scanJoinRow/scanLeftJoinRow below) -- never two
// separate queries.
type Row2[A any, B any] struct {
	A A
	B B
}

// Row3 is the typed composite result of a three-table join: A's, B's and
// C's scanned values side by side in one struct, populated by a single
// rows.Scan call (see scanJoinRow3 below). Row3 is DECLARED here because it
// is the shared result shape of Join3/JoinOn3 (see that type's doc comment
// for the nullability caveat that keeps it a plain unwrapped struct).
type Row3[A any, B any, C any] struct {
	A A
	B B
	C C
}

// JoinType selects which SQL JOIN keyword a join builder renders. INNER and
// LEFT are universal SQL; RIGHT and FULL are gated per-dialect via
// dialect.JoinCapabilities (see requireJoin) -- Postgres supports both,
// MySQL supports RIGHT only, and SQLite supports both from 3.39.0.
// A dialect lacking a keyword returns a typed
// dialect.ErrUnsupportedByDialect at execution time, never a silent
// substitution of a weaker join. Type alias for render.JoinType (orm
// imports render, never the reverse -- see orm.Op's doc comment for why
// aliasing beats a separately-kept-in-sync mirror); its String() method is
// defined on render.JoinType since a type alias cannot declare its own
// methods.
type JoinType = render.JoinType

// Supported join types. These re-export render's identically-named
// constants.
const (
	// InnerJoin renders "INNER JOIN"; a left row survives only when the
	// joined table has at least one matching row.
	InnerJoin = render.InnerJoin
	// LeftJoin renders "LEFT JOIN"; left rows without a match survive, with
	// the right side coming back all-NULL. Join2.All/Stream do NOT
	// distinguish that from a real all-zero-columns match (they always
	// return Row2[A, B]) -- use LeftJoinOn/LeftJoin2 for a null-safe result
	// (see LeftJoin2's doc comment).
	LeftJoin = render.LeftJoin
	// RightJoin renders "RIGHT JOIN"; right rows without a match survive,
	// with the LEFT side coming back all-NULL. Join2.All/Stream do NOT
	// distinguish that from a real match (they always return Row2[A, B]) --
	// use RightJoinOn/RightJoin2 for a null-safe result (see RightJoin2's
	// doc comment). Requires dialect.JoinCapabilities with
	// SupportsRightJoin reporting true.
	RightJoin = render.RightJoin
	// FullJoin renders "FULL JOIN"; either side without a match survives,
	// with the unmatched side coming back all-NULL. Join2.All/Stream do NOT
	// distinguish those from real matches (they always return Row2[A, B]) --
	// use FullJoinOn/FullJoin2 for a null-safe result (see FullJoin2's doc
	// comment). Requires dialect.JoinCapabilities with SupportsFullJoin
	// reporting true.
	FullJoin = render.FullJoin
)

// Join2 is an immutable, value-type two-table INNER/LEFT/RIGHT/FULL JOIN
// builder, following the same copy-on-write chain discipline as Query[T, PT].
//
// LEFT/RIGHT/FULL JOIN nullability design decision: Join2.All/
// Stream always return Row2[A, B] -- both sides are scanned directly through
// their normal pointer-receiver Scan methods, which errors if either side
// comes back all-NULL and any of its non-nullable Go fields would receive a
// raw SQL NULL. That makes Join2 with joinType == LeftJoin only safe to use
// when every one of B's non-key columns is itself a NullableColumn/Option
// field, joinType == RightJoin only safe when every one of A's is, and
// joinType == FullJoin only safe when BOTH sides' are, OR when the caller can
// prove the join can never actually miss. For the general "the joined side
// might not match" cases, use LeftJoinOn/RightJoinOn/FullJoinOn instead: they
// return LeftJoin2/RightJoin2/FullJoin2 whose All/Stream produce
// Row2[A, Option[B]] / Row2[Option[A], B] / Row2[Option[A], Option[B]] -- the
// WHOLE unmatched struct wrapped in Option, not per-field -- so an all-NULL
// side is unambiguously None, never a same-shaped zero struct. This
// two-type split (rather than one generic type whose return type depends on
// a runtime joinType value, which Go's type system cannot express) is the
// "separate method/type" resolution calls out as one acceptable
// shape.
type Join2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	rel        Relation[A, B]
	joinType   JoinType
	whereRight Predicate[B]
	orderRight []OrderTerm[B]
}

// JoinOn starts a Join2 from left through rel. joinType selects the
// rendered SQL join keyword; see Join2's doc comment for why joinType ==
// LeftJoin is only safe when B has no non-nullable columns that could
// legitimately come back NULL -- prefer LeftJoinOn for the general LEFT
// JOIN case (and RightJoinOn/FullJoinOn for the RIGHT/FULL ones).
func JoinOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], rel Relation[A, B], joinType JoinType) Join2[A, PA, B, PB] {
	return Join2[A, PA, B, PB]{left: left, rel: rel, joinType: joinType}
}

// requireJoin reports whether d can render jt. INNER and LEFT are universal
// SQL and never gated; RIGHT and FULL require dialect.JoinCapabilities with
// the matching Supports* method reporting true. A missing capability
// returns a typed dialect.ErrUnsupportedByDialect -- never a panic or a
// silent substitution of a weaker join ("never silently
// wrong SQL" rule).
func requireJoin(d dialect.Dialect, jt JoinType) error {
	if jt != RightJoin && jt != FullJoin {
		return nil
	}

	jd, ok := d.(dialect.JoinCapabilities)
	if !ok {
		return fmt.Errorf("orm: %w: dialect %q does not support %s", dialect.ErrUnsupportedByDialect, d.Name(), jt)
	}

	if jt == RightJoin && !jd.SupportsRightJoin() {
		return fmt.Errorf("orm: %w: dialect %q does not support RIGHT JOIN", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if jt == FullJoin && !jd.SupportsFullJoin() {
		return fmt.Errorf("orm: %w: dialect %q does not support FULL JOIN", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// Where combines p into j's left-table filter with AND, matching
// Query[T, PT].Where's copy-on-write rule exactly.
func (j Join2[A, PA, B, PB]) Where(p Predicate[A]) Join2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's right-table (B) filter with AND. It is a
// separate method from Where -- rather than one Where accepting either
// Predicate[A] or Predicate[B] -- specifically so A and B stay
// compile-time pinned at each call site.
func (j Join2[A, PA, B, PB]) WhereRight(p Predicate[B]) Join2[A, PA, B, PB] {
	if j.whereRight.IsSet() {
		j.whereRight = And(j.whereRight, p)
	} else {
		j.whereRight = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only, keeping
// each table's ordering statically scoped -- the ORDER BY analog of the
// Where/WhereRight split. Use OrderByRight for the right (B) table's
// columns; a caller needing both orders by both tables chains the two.
func (j Join2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) Join2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByRight appends ORDER BY terms scoped to the RIGHT (B) table only,
// mirroring the Where/WhereRight split: OrderBy stays left-scoped and
// OrderByRight right-scoped, so A and B stay compile-time pinned at each
// call site. The two lists render as one ORDER BY clause, left terms first,
// each qualified to its own table.
func (j Join2[A, PA, B, PB]) OrderByRight(terms ...OrderTerm[B]) Join2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderRight)+len(terms))
	next = append(next, j.orderRight...)
	next = append(next, terms...)
	j.orderRight = next

	return j
}

// Limit sets j's row limit.
func (j Join2[A, PA, B, PB]) Limit(n int) Join2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// render turns j's accumulated state into SQL text plus positional args.
// RIGHT/FULL JOIN are gated here (via requireJoin), so every execution path
// -- All, Stream and Explain -- rejects a dialect that lacks the keyword
// with the same typed error before any SQL is issued.
func (j Join2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	if err := requireJoin(d, j.joinType); err != nil {
		return "", nil, err
	}

	return render.SelectJoin(
		d,
		j.joinType,
		j.left.table.Name(), j.left.table.Columns(),
		j.rel.childTable.Name(), j.rel.childTable.Columns(),
		j.rel.parentCol, j.rel.childCol,
		toRenderNode[A](j.left.where.Render()),
		toRenderNode[B](j.whereRight.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderRight),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, B], scanned via ONE rows.Scan call per row spanning both A's and
// B's columns (see scanJoinRow) -- one round trip total, regardless of how
// many rows come back.
func (j Join2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, B], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: Join2.All: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	rows, err := queryRows(ctx, exec, query, encoded)
	if err != nil {
		return nil, fmt.Errorf("orm: Join2.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row2[A, B]

	aCols := len(j.left.table.Columns())
	bCols := len(j.rel.childTable.Columns())

	for rows.Next() {
		row, err := scanJoinRow[A, PA, B, PB](rows, aCols, bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: Join2.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: Join2.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: Join2.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time,
// via iter.Seq2[Row2[A, B], error] -- range-over-func's early-return
// cleanup closes the underlying rows cursor even if the consumer breaks
// out of the loop early, so nothing is leaked. This mirrors the shape
// describes for Query[T].Stream, applied here to Join2 since
// the four-type-parameter
// resolution above makes it a straightforward, mechanical extension of
// All's own per-row scanning.
func (j Join2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, B], error] {
	return func(yield func(Row2[A, B], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row2[A, B]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row2[A, B]{}, fmt.Errorf("orm: Join2.Stream: %w", err))

			return
		}

		encoded := encodeArgs(d, args)
		logQuery(query, encoded)

		rows, err := queryRows(ctx, exec, query, encoded)
		if err != nil {
			yield(Row2[A, B]{}, fmt.Errorf("orm: Join2.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.rel.childTable.Columns())

		for rows.Next() {
			row, err := scanJoinRow[A, PA, B, PB](rows, aCols, bCols)
			if err != nil {
				yield(Row2[A, B]{}, fmt.Errorf("orm: Join2.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row2[A, B]{}, fmt.Errorf("orm: Join2.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row2[A, B]{}, fmt.Errorf("orm: Join2.Stream: %w", err))
		}
	}
}

// LeftJoin2 is the null-safe counterpart to Join2 for LEFT JOINs -- see
// Join2's doc comment for the design decision. Its All/Stream return
// Row2[A, Option[B]]: an unmatched left row comes back with
// Option[B]{}.IsSome() == false, never a same-shaped zero-valued B{} that
// could be mistaken for a real all-zero-columns match.
type LeftJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	rel        Relation[A, B]
	whereRight Predicate[B]
	orderRight []OrderTerm[B]
}

// LeftJoinOn starts a LeftJoin2 from left through rel.
func LeftJoinOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], rel Relation[A, B]) LeftJoin2[A, PA, B, PB] {
	return LeftJoin2[A, PA, B, PB]{left: left, rel: rel}
}

// Where combines p into j's left-table filter with AND.
func (j LeftJoin2[A, PA, B, PB]) Where(p Predicate[A]) LeftJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's right-table (B) filter with AND. Note
// this filters the right side of a LEFT JOIN BEFORE the join is applied to
// the SQL a caller might expect from a plain WHERE -- it is rendered
// exactly like Join2.WhereRight (ANDed into the overall WHERE clause), so
// a set WhereRight predicate can turn an outer join back into something
// that behaves like an inner join for rows where B doesn't satisfy it.
// Callers who need "keep the parent row regardless" together with a
// right-side filter should filter client-side after Some(b) instead.
func (j LeftJoin2[A, PA, B, PB]) WhereRight(p Predicate[B]) LeftJoin2[A, PA, B, PB] {
	if j.whereRight.IsSet() {
		j.whereRight = And(j.whereRight, p)
	} else {
		j.whereRight = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only; see
// Join2.OrderBy's doc comment for why. Use OrderByRight for the right (B)
// table's columns.
func (j LeftJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) LeftJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByRight appends ORDER BY terms scoped to the RIGHT (B) table only;
// see Join2.OrderByRight for the Where/WhereRight-style split it mirrors.
func (j LeftJoin2[A, PA, B, PB]) OrderByRight(terms ...OrderTerm[B]) LeftJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderRight)+len(terms))
	next = append(next, j.orderRight...)
	next = append(next, terms...)
	j.orderRight = next

	return j
}

// Limit sets j's row limit.
func (j LeftJoin2[A, PA, B, PB]) Limit(n int) LeftJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

func (j LeftJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	return render.SelectJoin(
		d,
		render.LeftJoin,
		j.left.table.Name(), j.left.table.Columns(),
		j.rel.childTable.Name(), j.rel.childTable.Columns(),
		j.rel.parentCol, j.rel.childCol,
		toRenderNode[A](j.left.where.Render()),
		toRenderNode[B](j.whereRight.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderRight),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, Option[B]], scanned via ONE rows.Scan call per row (see
// scanLeftJoinRow) -- one round trip total.
func (j LeftJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, Option[B]], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: LeftJoin2.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: LeftJoin2.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row2[A, Option[B]]

	aCols := len(j.left.table.Columns())
	bCols := len(j.rel.childTable.Columns())

	for rows.Next() {
		row, err := scanLeftJoinRow[A, PA, B, PB](rows, aCols, bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: LeftJoin2.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: LeftJoin2.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: LeftJoin2.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j LeftJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, Option[B]], error] {
	return func(yield func(Row2[A, Option[B]], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row2[A, Option[B]]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row2[A, Option[B]]{}, fmt.Errorf("orm: LeftJoin2.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row2[A, Option[B]]{}, fmt.Errorf("orm: LeftJoin2.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.rel.childTable.Columns())

		for rows.Next() {
			row, err := scanLeftJoinRow[A, PA, B, PB](rows, aCols, bCols)
			if err != nil {
				yield(Row2[A, Option[B]]{}, fmt.Errorf("orm: LeftJoin2.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row2[A, Option[B]]{}, fmt.Errorf("orm: LeftJoin2.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row2[A, Option[B]]{}, fmt.Errorf("orm: LeftJoin2.Stream: %w", err))
		}
	}
}

// RightJoin2 is the null-safe counterpart to Join2 for RIGHT JOINs -- see
// Join2's doc comment for the design decision. Its All/Stream return
// Row2[Option[A], B]: an unmatched RIGHT-side row (a B with no matching A)
// comes back with Option[A]{}.IsSome() == false, never a same-shaped
// zero-valued A{} that could be mistaken for a real all-zero-columns match.
// B is scanned directly -- a RIGHT JOIN preserves every B row, so B's
// columns can never come back NULL. Requires a dialect with
// dialect.JoinCapabilities.SupportsRightJoin (see requireJoin).
type RightJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	rel        Relation[A, B]
	whereRight Predicate[B]
	orderRight []OrderTerm[B]
}

// RightJoinOn starts a RightJoin2 from left through rel.
func RightJoinOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], rel Relation[A, B]) RightJoin2[A, PA, B, PB] {
	return RightJoin2[A, PA, B, PB]{left: left, rel: rel}
}

// Where combines p into j's left-table filter with AND.
func (j RightJoin2[A, PA, B, PB]) Where(p Predicate[A]) RightJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's right-table (B) filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j RightJoin2[A, PA, B, PB]) WhereRight(p Predicate[B]) RightJoin2[A, PA, B, PB] {
	if j.whereRight.IsSet() {
		j.whereRight = And(j.whereRight, p)
	} else {
		j.whereRight = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only; see
// Join2.OrderBy. Use OrderByRight for the right (B) table's columns.
func (j RightJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) RightJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByRight appends ORDER BY terms scoped to the RIGHT (B) table only;
// see Join2.OrderByRight for the Where/WhereRight-style split it mirrors.
func (j RightJoin2[A, PA, B, PB]) OrderByRight(terms ...OrderTerm[B]) RightJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderRight)+len(terms))
	next = append(next, j.orderRight...)
	next = append(next, terms...)
	j.orderRight = next

	return j
}

// Limit sets j's row limit.
func (j RightJoin2[A, PA, B, PB]) Limit(n int) RightJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// render turns j's accumulated state into SQL text plus positional args.
// RIGHT JOIN is gated here via requireJoin, so All/Stream/Explain all
// reject a dialect that lacks it with the same typed error.
func (j RightJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	if err := requireJoin(d, RightJoin); err != nil {
		return "", nil, err
	}

	return render.SelectJoin(
		d,
		render.RightJoin,
		j.left.table.Name(), j.left.table.Columns(),
		j.rel.childTable.Name(), j.rel.childTable.Columns(),
		j.rel.parentCol, j.rel.childCol,
		toRenderNode[A](j.left.where.Render()),
		toRenderNode[B](j.whereRight.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderRight),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[Option[A], B], scanned via ONE rows.Scan call per row (see
// scanRightJoinRow) -- one round trip total.
func (j RightJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[Option[A], B], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: RightJoin2.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: RightJoin2.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row2[Option[A], B]

	aCols := len(j.left.table.Columns())
	bCols := len(j.rel.childTable.Columns())

	for rows.Next() {
		row, err := scanRightJoinRow[A, PA, B, PB](rows, aCols, bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: RightJoin2.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: RightJoin2.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: RightJoin2.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j RightJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[Option[A], B], error] {
	return func(yield func(Row2[Option[A], B], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row2[Option[A], B]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row2[Option[A], B]{}, fmt.Errorf("orm: RightJoin2.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row2[Option[A], B]{}, fmt.Errorf("orm: RightJoin2.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.rel.childTable.Columns())

		for rows.Next() {
			row, err := scanRightJoinRow[A, PA, B, PB](rows, aCols, bCols)
			if err != nil {
				yield(Row2[Option[A], B]{}, fmt.Errorf("orm: RightJoin2.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row2[Option[A], B]{}, fmt.Errorf("orm: RightJoin2.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row2[Option[A], B]{}, fmt.Errorf("orm: RightJoin2.Stream: %w", err))
		}
	}
}

// FullJoin2 is the null-safe counterpart to Join2 for FULL JOINs -- see
// Join2's doc comment for the design decision. Its All/Stream return
// Row2[Option[A], Option[B]]: a FULL JOIN preserves BOTH unmatched sides,
// so either half of a row can come back all-NULL, and each is wrapped in
// its own Option -- an unmatched left row has B = None, an unmatched right
// row has A = None, unambiguously never a same-shaped zero struct. Requires
// a dialect with dialect.JoinCapabilities.SupportsFullJoin (see requireJoin).
type FullJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	rel        Relation[A, B]
	whereRight Predicate[B]
	orderRight []OrderTerm[B]
}

// FullJoinOn starts a FullJoin2 from left through rel.
func FullJoinOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], rel Relation[A, B]) FullJoin2[A, PA, B, PB] {
	return FullJoin2[A, PA, B, PB]{left: left, rel: rel}
}

// Where combines p into j's left-table filter with AND.
func (j FullJoin2[A, PA, B, PB]) Where(p Predicate[A]) FullJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's right-table (B) filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j FullJoin2[A, PA, B, PB]) WhereRight(p Predicate[B]) FullJoin2[A, PA, B, PB] {
	if j.whereRight.IsSet() {
		j.whereRight = And(j.whereRight, p)
	} else {
		j.whereRight = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only; see
// Join2.OrderBy. Use OrderByRight for the right (B) table's columns.
func (j FullJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) FullJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByRight appends ORDER BY terms scoped to the RIGHT (B) table only;
// see Join2.OrderByRight for the Where/WhereRight-style split it mirrors.
func (j FullJoin2[A, PA, B, PB]) OrderByRight(terms ...OrderTerm[B]) FullJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderRight)+len(terms))
	next = append(next, j.orderRight...)
	next = append(next, terms...)
	j.orderRight = next

	return j
}

// Limit sets j's row limit.
func (j FullJoin2[A, PA, B, PB]) Limit(n int) FullJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// render turns j's accumulated state into SQL text plus positional args.
// FULL JOIN is gated here via requireJoin, so All/Stream/Explain all
// reject a dialect that lacks it with the same typed error.
func (j FullJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	if err := requireJoin(d, FullJoin); err != nil {
		return "", nil, err
	}

	return render.SelectJoin(
		d,
		render.FullJoin,
		j.left.table.Name(), j.left.table.Columns(),
		j.rel.childTable.Name(), j.rel.childTable.Columns(),
		j.rel.parentCol, j.rel.childCol,
		toRenderNode[A](j.left.where.Render()),
		toRenderNode[B](j.whereRight.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderRight),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[Option[A], Option[B]], scanned via ONE rows.Scan call per row (see
// scanFullJoinRow) -- one round trip total.
func (j FullJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[Option[A], Option[B]], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: FullJoin2.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: FullJoin2.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row2[Option[A], Option[B]]

	aCols := len(j.left.table.Columns())
	bCols := len(j.rel.childTable.Columns())

	for rows.Next() {
		row, err := scanFullJoinRow[A, PA, B, PB](rows, aCols, bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: FullJoin2.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: FullJoin2.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: FullJoin2.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j FullJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[Option[A], Option[B]], error] {
	return func(yield func(Row2[Option[A], Option[B]], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row2[Option[A], Option[B]]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: FullJoin2.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: FullJoin2.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.rel.childTable.Columns())

		for rows.Next() {
			row, err := scanFullJoinRow[A, PA, B, PB](rows, aCols, bCols)
			if err != nil {
				yield(Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: FullJoin2.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: FullJoin2.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: FullJoin2.Stream: %w", err))
		}
	}
}

// Join3 is an immutable, value-type three-table join builder following the
// same copy-on-write chain discipline as Join2. It joins A through relAB to
// B, then B through relBC to C, rendering ONE SELECT with all three tables'
// columns and TWO JOIN clauses of the same joinType, scanned via ONE
// rows.Scan call per row into a Row3[A, B, C] (see scanJoinRow3).
//
// Nullability: All/Stream return Row3[A, B, C] with all three sides scanned
// directly through their normal pointer-receiver Scan methods, which is only
// safe for joinType == InnerJoin, or when the caller can prove every non-key
// column on every side is itself a NullableColumn/Option field. For a
// null-safe three-table join use the dedicated outer builders, which wrap
// the optional sides in Option: LeftJoinOn3 (Row3[A, Option[B], Option[C]]),
// RightJoinOn3 (Row3[Option[A], Option[B], C]), FullJoinOn3 (all three
// Option), and the mixed-kind InnerLeftJoinOn3 / LeftInnerJoinOn3 (see
// join3_mixed.go).
type Join3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
	left     Query[A, PA]
	relAB    Relation[A, B]
	relBC    Relation[B, C]
	joinType JoinType
	whereB   Predicate[B]
	whereC   Predicate[C]
	order    []OrderTerm[A]
	orderB   []OrderTerm[B]
	orderC   []OrderTerm[C]
	limit    int
	offset   int
}

// JoinOn3 starts a Join3 from left through relAB to B, then relBC to C.
// joinType applies to BOTH joins; see Join3's doc comment for why only
// InnerJoin (or a caller-provable non-null join) is safe with the unwrapped
// Row3 result.
func JoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
	joinType JoinType,
) Join3[A, PA, B, PB, C, PC] {
	return Join3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC, joinType: joinType}
}

// Where combines p into j's A-side filter with AND.
func (j Join3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) Join3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side (the first right table) filter with
// AND, extending Join2.WhereRight's B-scoped meaning one table further.
func (j Join3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) Join3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side (the rightmost table) filter with AND.
func (j Join3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) Join3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only; see
// Join2.OrderBy. Use OrderByRight/OrderByC for B's and C's columns.
func (j Join3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) Join3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only; see
// Join2.OrderByRight for the Where/WhereRight-style split it mirrors.
func (j Join3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) Join3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C (rightmost) table only.
func (j Join3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) Join3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j Join3[A, PA, B, PB, C, PC]) Limit(n int) Join3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j Join3[A, PA, B, PB, C, PC]) Offset(n int) Join3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// renderJoin3 renders one three-table joined SELECT for the HOMOGENEOUS
// (#409) builders, applying joinType to BOTH hops. It delegates to
// renderJoin3Mixed, so every three-table render path -- homogeneous and
// mixed -- shares one body and can never drift apart.
func renderJoin3[
	A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C],
](
	d dialect.Dialect,
	joinType JoinType,
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
	whereB Predicate[B],
	whereC Predicate[C],
	orderA []OrderTerm[A],
	orderB []OrderTerm[B],
	orderC []OrderTerm[C],
	limit, offset int,
) (string, []any, error) {
	return renderJoin3Mixed[A, PA, B, PB, C, PC](d, joinType, joinType, false, left, relAB, relBC, whereB, whereC, orderA, orderB, orderC, limit, offset)
}

// renderJoin3Mixed renders one three-table joined SELECT from an
// already-built Join3-family builder's parts: it applies the join-type
// capability gate (requireJoin) to EACH hop individually, then calls
// render.SelectJoin3Mixed or render.SelectJoin3Nested, so all three-table
// builders -- homogeneous and mixed -- share one render body and can never
// drift apart. A RIGHT/FULL hop is rejected here with the matching typed
// dialect.ErrUnsupportedByDialect before any SQL is issued. nestedBC
// parenthesizes the B/C pair as a composite (SelectJoin3Nested) rather than
// rendering the flat, left-associative chain (SelectJoin3Mixed).
func renderJoin3Mixed[
	A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C],
](
	d dialect.Dialect,
	joinAB, joinBC JoinType,
	nestedBC bool,
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
	whereB Predicate[B],
	whereC Predicate[C],
	orderA []OrderTerm[A],
	orderB []OrderTerm[B],
	orderC []OrderTerm[C],
	limit, offset int,
) (string, []any, error) {
	if err := requireJoin(d, joinAB); err != nil {
		return "", nil, err
	}

	if err := requireJoin(d, joinBC); err != nil {
		return "", nil, err
	}

	if nestedBC {
		return render.SelectJoin3Nested(
			d,
			joinAB, joinBC,
			left.table.Name(), left.table.Columns(),
			relAB.childTable.Name(), relAB.childTable.Columns(),
			relBC.childTable.Name(), relBC.childTable.Columns(),
			relAB.parentCol, relAB.childCol,
			relBC.parentCol, relBC.childCol,
			toRenderNode[A](left.where.Render()),
			toRenderNode[B](whereB.Render()),
			toRenderNode[C](whereC.Render()),
			toRenderOrder(orderA),
			toRenderOrder(orderB),
			toRenderOrder(orderC),
			limit, offset,
		)
	}

	return render.SelectJoin3Mixed(
		d,
		joinAB, joinBC,
		left.table.Name(), left.table.Columns(),
		relAB.childTable.Name(), relAB.childTable.Columns(),
		relBC.childTable.Name(), relBC.childTable.Columns(),
		relAB.parentCol, relAB.childCol,
		relBC.parentCol, relBC.childCol,
		toRenderNode[A](left.where.Render()),
		toRenderNode[B](whereB.Render()),
		toRenderNode[C](whereC.Render()),
		toRenderOrder(orderA),
		toRenderOrder(orderB),
		toRenderOrder(orderC),
		limit, offset,
	)
}

// render turns j's accumulated state into SQL text plus positional args.
// A non-inner joinType is gated here via requireJoin.
func (j Join3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3[A, PA, B, PB, C, PC](d, j.joinType, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[A, B, C], scanned via ONE rows.Scan call per row (see scanJoinRow3)
// -- one round trip total, regardless of how many rows come back.
func (j Join3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[A, B, C], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: Join3.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: Join3.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row3[A, B, C]

	aCols := len(j.left.table.Columns())
	bCols := len(j.relAB.childTable.Columns())
	cCols := len(j.relBC.childTable.Columns())

	for rows.Next() {
		row, err := scanJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
		if err != nil {
			return nil, fmt.Errorf("orm: Join3.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: Join3.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: Join3.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time,
// via iter.Seq2[Row3[A, B, C], error]. See Join2.Stream's doc comment for
// the range-over-func cleanup guarantee.
func (j Join3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[A, B, C], error] {
	return func(yield func(Row3[A, B, C], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row3[A, B, C]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row3[A, B, C]{}, fmt.Errorf("orm: Join3.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row3[A, B, C]{}, fmt.Errorf("orm: Join3.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.relAB.childTable.Columns())
		cCols := len(j.relBC.childTable.Columns())

		for rows.Next() {
			row, err := scanJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
			if err != nil {
				yield(Row3[A, B, C]{}, fmt.Errorf("orm: Join3.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row3[A, B, C]{}, fmt.Errorf("orm: Join3.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row3[A, B, C]{}, fmt.Errorf("orm: Join3.Stream: %w", err))
		}
	}
}

// LeftJoin3 is the null-safe three-table LEFT JOIN builder: it chains
// "A LEFT JOIN B ON ... LEFT JOIN C ON ..." and returns
// Row3[A, Option[B], Option[C]].
//
// Nullability semantics of a chained LEFT JOIN (this is why a single
// Option on the "joined" side is NOT enough, the way Row2[A, Option[B]] is
// for the two-table case): A is always present; B is present iff some B
// matched A; C is present iff some C matched a present B. An absent B
// therefore forces an absent C -- the LEFT-chain invariant
// C.IsSome() implies B.IsSome() -- so the result carries one Option per
// joined side and the caller reads B first, then C. Neither Option is ever a
// same-shaped zero struct: an unmatched side is unambiguously None, just as
// LeftJoin2 guarantees for one table.
//
// Every dialect supports the LEFT keyword, so LeftJoin3 is ungated.
type LeftJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
	left   Query[A, PA]
	relAB  Relation[A, B]
	relBC  Relation[B, C]
	whereB Predicate[B]
	whereC Predicate[C]
	order  []OrderTerm[A]
	orderB []OrderTerm[B]
	orderC []OrderTerm[C]
	limit  int
	offset int
}

// LeftJoinOn3 starts a LeftJoin3 from left through relAB to B, then relBC
// to C -- the three-table counterpart of LeftJoinOn.
func LeftJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
) LeftJoin3[A, PA, B, PB, C, PC] {
	return LeftJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC}
}

// Where combines p into j's A-side filter with AND.
func (j LeftJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) LeftJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j LeftJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) LeftJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j LeftJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) LeftJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j LeftJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) LeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j LeftJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) LeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j LeftJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) LeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j LeftJoin3[A, PA, B, PB, C, PC]) Limit(n int) LeftJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j LeftJoin3[A, PA, B, PB, C, PC]) Offset(n int) LeftJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

func (j LeftJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3[A, PA, B, PB, C, PC](d, LeftJoin, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[A, Option[B], Option[C]], scanned via ONE rows.Scan call per row (see
// scanLeftJoinRow3) -- one round trip total.
func (j LeftJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[A, Option[B], Option[C]], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: LeftJoin3.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: LeftJoin3.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row3[A, Option[B], Option[C]]

	aCols := len(j.left.table.Columns())
	bCols := len(j.relAB.childTable.Columns())
	cCols := len(j.relBC.childTable.Columns())

	for rows.Next() {
		row, err := scanLeftJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
		if err != nil {
			return nil, fmt.Errorf("orm: LeftJoin3.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: LeftJoin3.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: LeftJoin3.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j LeftJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[A, Option[B], Option[C]], error] {
	return func(yield func(Row3[A, Option[B], Option[C]], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row3[A, Option[B], Option[C]]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: LeftJoin3.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: LeftJoin3.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.relAB.childTable.Columns())
		cCols := len(j.relBC.childTable.Columns())

		for rows.Next() {
			row, err := scanLeftJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
			if err != nil {
				yield(Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: LeftJoin3.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: LeftJoin3.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: LeftJoin3.Stream: %w", err))
		}
	}
}

// RightJoin3 is the null-safe three-table RIGHT JOIN builder: it chains
// "A RIGHT JOIN B ON ... RIGHT JOIN C ON ..." and returns
// Row3[Option[A], Option[B], C].
//
// A chained RIGHT JOIN preserves every C row, so C is always present. B is
// present iff some B matched the C row; A is present iff that B also matched
// an A -- so the RIGHT-chain invariant is A.IsSome() implies B.IsSome(), and
// an absent B forces an absent A. Both optional sides are wrapped in their
// own Option so an unmatched side is unambiguously None, never a same-shaped
// zero struct (mirroring RightJoin2). Requires a dialect with
// dialect.JoinCapabilities.SupportsRightJoin (see requireJoin).
type RightJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
	left   Query[A, PA]
	relAB  Relation[A, B]
	relBC  Relation[B, C]
	whereB Predicate[B]
	whereC Predicate[C]
	order  []OrderTerm[A]
	orderB []OrderTerm[B]
	orderC []OrderTerm[C]
	limit  int
	offset int
}

// RightJoinOn3 starts a RightJoin3 from left through relAB to B, then relBC
// to C -- the three-table counterpart of RightJoinOn.
func RightJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
) RightJoin3[A, PA, B, PB, C, PC] {
	return RightJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC}
}

// Where combines p into j's A-side filter with AND.
func (j RightJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) RightJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j RightJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) RightJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j RightJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) RightJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j RightJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) RightJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j RightJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) RightJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j RightJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) RightJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j RightJoin3[A, PA, B, PB, C, PC]) Limit(n int) RightJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j RightJoin3[A, PA, B, PB, C, PC]) Offset(n int) RightJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// render turns j's accumulated state into SQL text plus positional args.
// RIGHT JOIN is gated here via requireJoin, so All/Stream reject a dialect
// that lacks it with the same typed error.
func (j RightJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3[A, PA, B, PB, C, PC](d, RightJoin, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[Option[A], Option[B], C], scanned via ONE rows.Scan call per row (see
// scanRightJoinRow3) -- one round trip total.
func (j RightJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[Option[A], Option[B], C], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: RightJoin3.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: RightJoin3.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row3[Option[A], Option[B], C]

	aCols := len(j.left.table.Columns())
	bCols := len(j.relAB.childTable.Columns())
	cCols := len(j.relBC.childTable.Columns())

	for rows.Next() {
		row, err := scanRightJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
		if err != nil {
			return nil, fmt.Errorf("orm: RightJoin3.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: RightJoin3.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: RightJoin3.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j RightJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[Option[A], Option[B], C], error] {
	return func(yield func(Row3[Option[A], Option[B], C], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row3[Option[A], Option[B], C]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: RightJoin3.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: RightJoin3.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.relAB.childTable.Columns())
		cCols := len(j.relBC.childTable.Columns())

		for rows.Next() {
			row, err := scanRightJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
			if err != nil {
				yield(Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: RightJoin3.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: RightJoin3.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: RightJoin3.Stream: %w", err))
		}
	}
}

// FullJoin3 is the null-safe three-table FULL JOIN builder: it chains
// "A FULL JOIN B ON ... FULL JOIN C ON ..." and returns
// Row3[Option[A], Option[B], Option[C]].
//
// A FULL JOIN preserves every unmatched side, so any of the three halves can
// come back all-NULL and each is wrapped in its own Option -- an unmatched
// side is unambiguously None, never a same-shaped zero struct (mirroring
// FullJoin2). Because the second join keys on B, a present C still implies a
// present B (the FULL-chain invariant C.IsSome() implies B.IsSome()); A is
// independent of both. Requires a dialect with
// dialect.JoinCapabilities.SupportsFullJoin (see requireJoin).
type FullJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
	left   Query[A, PA]
	relAB  Relation[A, B]
	relBC  Relation[B, C]
	whereB Predicate[B]
	whereC Predicate[C]
	order  []OrderTerm[A]
	orderB []OrderTerm[B]
	orderC []OrderTerm[C]
	limit  int
	offset int
}

// FullJoinOn3 starts a FullJoin3 from left through relAB to B, then relBC
// to C -- the three-table counterpart of FullJoinOn.
func FullJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
) FullJoin3[A, PA, B, PB, C, PC] {
	return FullJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC}
}

// Where combines p into j's A-side filter with AND.
func (j FullJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) FullJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j FullJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) FullJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically here.
func (j FullJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) FullJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j FullJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) FullJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j FullJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) FullJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j FullJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) FullJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j FullJoin3[A, PA, B, PB, C, PC]) Limit(n int) FullJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j FullJoin3[A, PA, B, PB, C, PC]) Offset(n int) FullJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// render turns j's accumulated state into SQL text plus positional args.
// FULL JOIN is gated here via requireJoin, so All/Stream reject a dialect
// that lacks it with the same typed error.
func (j FullJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3[A, PA, B, PB, C, PC](d, FullJoin, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[Option[A], Option[B], Option[C]], scanned via ONE rows.Scan call per
// row (see scanFullJoinRow3) -- one round trip total.
func (j FullJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[Option[A], Option[B], Option[C]], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: FullJoin3.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: FullJoin3.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row3[Option[A], Option[B], Option[C]]

	aCols := len(j.left.table.Columns())
	bCols := len(j.relAB.childTable.Columns())
	cCols := len(j.relBC.childTable.Columns())

	for rows.Next() {
		row, err := scanFullJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
		if err != nil {
			return nil, fmt.Errorf("orm: FullJoin3.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: FullJoin3.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: FullJoin3.All: %w", err)
	}

	return out, nil
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j FullJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[Option[A], Option[B], Option[C]], error] {
	return func(yield func(Row3[Option[A], Option[B], Option[C]], error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(Row3[Option[A], Option[B], Option[C]]{}, err)

			return
		}

		query, args, err := j.render(d)
		if err != nil {
			yield(Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: FullJoin3.Stream: %w", err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: FullJoin3.Stream: %w", err))

			return
		}
		defer func() { _ = rows.Close() }()

		aCols := len(j.left.table.Columns())
		bCols := len(j.relAB.childTable.Columns())
		cCols := len(j.relBC.childTable.Columns())

		for rows.Next() {
			row, err := scanFullJoinRow3[A, PA, B, PB, C, PC](rows, aCols, bCols, cCols)
			if err != nil {
				yield(Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: FullJoin3.Stream: scan: %w", err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: FullJoin3.Stream: %w", err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: FullJoin3.Stream: %w", err))
		}
	}
}

// LateralJoin2 is an immutable, value-type CROSS JOIN LATERAL builder: a
// LATERAL join of inner onto left (a Query[A, PA] over the LEFT/outer
// table), rendered as `FROM A CROSS JOIN LATERAL (inner) AS alias`. It is
// the "top-N per group" shape -- e.g. the two most recent orders per user:
//
//	topOrders := orm.From(Orders).Where(Orders.UserID.EqOuter(orm.Outer(Users.ID))).
//		OrderBy(Orders.PlacedAt.Desc()).Limit(2)
//
//	rows, err := orm.CrossLateralOn(orm.From(Users), topOrders).
//		OrderBy(orm.Users.ID.Asc()).All(ctx, conn)
//
// The inner query is snapshotted (value semantics) at construction time and
// rendered through the ENCLOSING statement's dialect at execution time, with
// a orm.Outer/OuterNullable marker in its Where resolving against left's
// table (see orm/render.SelectLateral). The inner query's own ORDER BY/LIMIT
// render INSIDE the lateral parentheses, which is what makes it a per-outer-
// row limit rather than a global one.
//
// All/Stream return Row2[A, B], scanned via ONE rows.Scan call per row (see
// scanJoinRow): a CROSS JOIN LATERAL emits a row only when the inner query
// matches, so B is always present. For the general "keep the outer row even
// when the inner query matches nothing" case use LeftLateralOn.
//
// The inner query must project its entity's full canonical column list (the
// default); an explicit Query.Columns projection that differs is rejected at
// render time with a typed error, because B is scanned positionally. LATERAL
// itself is gated by dialect.LateralJoinDialect: Postgres supports it,
// SQLite does not (a typed
// dialect.ErrUnsupportedByDialect, never a silently-degraded join).
type LateralJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	inner      Query[B, PB]
	orderInner []OrderTerm[B]
}

// CrossLateralOn starts a LateralJoin2 from left over the lateral inner
// query. See LateralJoin2 for the top-N-per-group idiom and the result and
// capability contract.
func CrossLateralOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], inner Query[B, PB]) LateralJoin2[A, PA, B, PB] {
	return LateralJoin2[A, PA, B, PB]{left: left, inner: inner}
}

// Where combines p into j's left-table filter with AND, matching
// Query[T, PT].Where's copy-on-write rule exactly. To filter the inner
// (lateral) query, build the filter into the inner Query passed to
// CrossLateralOn -- it is a normal Query, not a restricted subquery.
func (j LateralJoin2[A, PA, B, PB]) Where(p Predicate[A]) LateralJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only. Use
// OrderByInner for the lateral subquery's columns; the two lists render as
// one ORDER BY clause, left terms first, each qualified to its own side.
func (j LateralJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) LateralJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByInner appends ORDER BY terms scoped to the lateral subquery's
// projected columns (qualified with the derived-table alias), mirroring the
// Where/WhereRight-style split: A stays compile-time pinned to the outer
// side and B to the inner.
func (j LateralJoin2[A, PA, B, PB]) OrderByInner(terms ...OrderTerm[B]) LateralJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderInner)+len(terms))
	next = append(next, j.orderInner...)
	next = append(next, terms...)
	j.orderInner = next

	return j
}

// Limit sets j's outer row limit.
func (j LateralJoin2[A, PA, B, PB]) Limit(n int) LateralJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// Offset sets j's outer row offset.
func (j LateralJoin2[A, PA, B, PB]) Offset(n int) LateralJoin2[A, PA, B, PB] {
	j.left = j.left.Offset(n)

	return j
}

func (j LateralJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	if err := requireLateral(d); err != nil {
		return "", nil, err
	}

	inner, err := lateralInner(j.inner)
	if err != nil {
		return "", nil, fmt.Errorf("orm: LateralJoin2: %w", err)
	}

	return render.SelectLateral(
		d, true,
		j.left.table.Name(), j.left.table.Columns(),
		inner,
		lateralAlias(j.left.table.Name(), j.inner.table.Name()),
		toRenderNode[A](j.left.where.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderInner),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, B], scanned via ONE rows.Scan call per row (see scanJoinRow) --
// one round trip total.
func (j LateralJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, B], error) {
	return lateralCollect(ctx, exec, "LateralJoin2.All", j.render,
		len(j.left.table.Columns()), len(j.inner.table.Columns()),
		scanJoinRow[A, PA, B, PB])
}

// Stream runs j against exec and yields every matching row one at a time,
// via iter.Seq2[Row2[A, B], error]. See Join2.Stream for the range-over-func
// cleanup guarantee.
func (j LateralJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, B], error] {
	return lateralStream(ctx, exec, "LateralJoin2.Stream", j.render,
		len(j.left.table.Columns()), len(j.inner.table.Columns()),
		scanJoinRow[A, PA, B, PB])
}

// Explain runs j's lateral join SELECT under EXPLAIN against exec; see
// Query.Explain for the passthrough contract. The dialect is gated by
// render (dialect.LateralJoinDialect), so a dialect without LATERAL is a
// typed ErrUnsupportedByDialect here too.
func (j LateralJoin2[A, PA, B, PB]) Explain(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, false, "LateralJoin2.Explain", j.render)
}

// ExplainAnalyze runs j's lateral join SELECT under EXPLAIN ANALYZE against
// exec; see Query.ExplainAnalyze for the capability gate.
func (j LateralJoin2[A, PA, B, PB]) ExplainAnalyze(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, true, "LateralJoin2.Explain", j.render)
}

// LeftLateralJoin2 is the null-safe sibling of LateralJoin2 for
// `LEFT JOIN LATERAL (...) ON TRUE`: All/Stream return Row2[A, Option[B]],
// so an outer row whose lateral subquery matches nothing comes back with
// Option[B]{}.IsSome() == false, never a same-shaped zero-valued B{} that
// could be mistaken for a real all-zero-columns match.
type LeftLateralJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left       Query[A, PA]
	inner      Query[B, PB]
	orderInner []OrderTerm[B]
}

// LeftLateralOn starts a LeftLateralJoin2 from left over the lateral inner
// query, rendered as `FROM A LEFT JOIN LATERAL (inner) AS alias ON TRUE`. It
// is the null-safe "optionally-correlated rows" shape: every outer row
// survives even when the inner query matches nothing. See CrossLateralOn for
// the correlation, snapshotting and capability contract; the inner query's
// projection constraint applies here too.
func LeftLateralOn[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], inner Query[B, PB]) LeftLateralJoin2[A, PA, B, PB] {
	return LeftLateralJoin2[A, PA, B, PB]{left: left, inner: inner}
}

// Where combines p into j's left-table filter with AND.
func (j LeftLateralJoin2[A, PA, B, PB]) Where(p Predicate[A]) LeftLateralJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only; use
// OrderByInner for the lateral subquery's columns.
func (j LeftLateralJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) LeftLateralJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderByInner appends ORDER BY terms scoped to the lateral subquery's
// projected columns; see LateralJoin2.OrderByInner.
func (j LeftLateralJoin2[A, PA, B, PB]) OrderByInner(terms ...OrderTerm[B]) LeftLateralJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderInner)+len(terms))
	next = append(next, j.orderInner...)
	next = append(next, terms...)
	j.orderInner = next

	return j
}

// Limit sets j's outer row limit.
func (j LeftLateralJoin2[A, PA, B, PB]) Limit(n int) LeftLateralJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// Offset sets j's outer row offset.
func (j LeftLateralJoin2[A, PA, B, PB]) Offset(n int) LeftLateralJoin2[A, PA, B, PB] {
	j.left = j.left.Offset(n)

	return j
}

func (j LeftLateralJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	if err := requireLateral(d); err != nil {
		return "", nil, err
	}

	inner, err := lateralInner(j.inner)
	if err != nil {
		return "", nil, fmt.Errorf("orm: LeftLateralJoin2: %w", err)
	}

	return render.SelectLateral(
		d, false,
		j.left.table.Name(), j.left.table.Columns(),
		inner,
		lateralAlias(j.left.table.Name(), j.inner.table.Name()),
		toRenderNode[A](j.left.where.Render()),
		toRenderOrder(j.left.order),
		toRenderOrder(j.orderInner),
		j.left.limit, j.left.offset,
	)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, Option[B]], scanned via ONE rows.Scan call per row (see
// scanLeftJoinRow) -- one round trip total.
func (j LeftLateralJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, Option[B]], error) {
	return lateralCollect(ctx, exec, "LeftLateralJoin2.All", j.render,
		len(j.left.table.Columns()), len(j.inner.table.Columns()),
		scanLeftJoinRow[A, PA, B, PB])
}

// Stream runs j against exec and yields every matching row one at a time,
// via iter.Seq2[Row2[A, Option[B]], error].
func (j LeftLateralJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, Option[B]], error] {
	return lateralStream(ctx, exec, "LeftLateralJoin2.Stream", j.render,
		len(j.left.table.Columns()), len(j.inner.table.Columns()),
		scanLeftJoinRow[A, PA, B, PB])
}

// Explain runs j's LEFT JOIN LATERAL SELECT under EXPLAIN against exec; see
// Query.Explain and LateralJoin2.Explain for the dialect gate.
func (j LeftLateralJoin2[A, PA, B, PB]) Explain(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, false, "LeftLateralJoin2.Explain", j.render)
}

// ExplainAnalyze runs j's LEFT JOIN LATERAL SELECT under EXPLAIN ANALYZE
// against exec; see Query.ExplainAnalyze for the capability gate.
func (j LeftLateralJoin2[A, PA, B, PB]) ExplainAnalyze(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, true, "LeftLateralJoin2.Explain", j.render)
}

// lateralCollect runs a rendered lateral SELECT and scans each result row
// through scan into a slice of Row. It is the shared body behind the four
// lateral All methods, so the CROSS and LEFT variants cannot drift; the
// typed per-side Option is supplied entirely by the scan function each
// caller passes (scanJoinRow vs scanLeftJoinRow).
func lateralCollect[Row any](
	ctx context.Context,
	exec db.DB,
	tag string,
	renderFn func(dialect.Dialect) (string, []any, error),
	aCols, bCols int,
	scan func(rows db.Rows, aCols, bCols int) (Row, error),
) ([]Row, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := renderFn(d)
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	rows, err := queryRows(ctx, exec, query, encoded)
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row

	for rows.Next() {
		row, err := scan(rows, aCols, bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: %s: scan: %w", tag, err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	return out, nil
}

// lateralStream is the iter.Seq2 counterpart to lateralCollect: it renders
// the lateral SELECT once and yields each scanned row, closing the cursor on
// early consumer exit via range-over-func's cleanup contract (see
// Join2.Stream). The four lateral Stream methods share it.
func lateralStream[Row any](
	ctx context.Context,
	exec db.DB,
	tag string,
	renderFn func(dialect.Dialect) (string, []any, error),
	aCols, bCols int,
	scan func(rows db.Rows, aCols, bCols int) (Row, error),
) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		var zero Row

		d, err := resolveDialect(exec)
		if err != nil {
			yield(zero, err)

			return
		}

		query, args, err := renderFn(d)
		if err != nil {
			yield(zero, fmt.Errorf("orm: %s: %w", tag, err))

			return
		}

		encoded := encodeArgs(d, args)
		logQuery(query, encoded)

		rows, err := queryRows(ctx, exec, query, encoded)
		if err != nil {
			yield(zero, fmt.Errorf("orm: %s: %w", tag, err))

			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			row, err := scan(rows, aCols, bCols)
			if err != nil {
				yield(zero, fmt.Errorf("orm: %s: scan: %w", tag, err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(zero, fmt.Errorf("orm: %s: %w", tag, err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(zero, fmt.Errorf("orm: %s: %w", tag, err))
		}
	}
}

// requireLateral reports whether d supports a LATERAL derived-table join.
// A dialect without dialect.LateralJoinDialect -- or one whose
// SupportsLateral reports false (every SQLite) -- is
// rejected with a typed dialect.ErrUnsupportedByDialect, never a panic or a
// silently-degraded join.
func requireLateral(d dialect.Dialect) error {
	ld, ok := d.(dialect.LateralJoinDialect)
	if !ok || !ld.SupportsLateral() {
		return fmt.Errorf("orm: %w: dialect %q does not support LATERAL joins", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// lateralAlias names the lateral derived table. The inner table's own name
// is used so the emitted SQL reads naturally; when the inner table IS the
// outer table (a self-lateral), the alias is suffixed so the two FROM
// entries never collide.
func lateralAlias(leftTable, innerTable string) string {
	if innerTable == leftTable {
		return innerTable + "_lateral"
	}

	return innerTable
}

// lateralInner snapshots inner into render's erased Subquery and enforces
// the projection invariant the typed Row2 scan depends on: the subquery must
// project exactly the entity's canonical column list, in order, because B's
// codegen'd Scan reads positionally. An explicit Query.Columns projection
// that differs is a typed construction-time error rather than a mismatched
// scan at execution time.
func lateralInner[B any, PB ptrScanner[B]](inner Query[B, PB]) (render.Subquery, error) {
	sq := toSubquery(inner)

	tableCols := inner.table.Columns()
	if len(sq.columns) != len(tableCols) {
		return render.Subquery{}, fmt.Errorf("lateral subquery must project entity %q's full column list (%d), got %d",
			inner.table.Name(), len(tableCols), len(sq.columns))
	}

	for i := range tableCols {
		if sq.columns[i] != tableCols[i] {
			return render.Subquery{}, fmt.Errorf("lateral subquery must project entity %q's full column list in order; column %d is %q, want %q",
				inner.table.Name(), i, sq.columns[i], tableCols[i])
		}
	}

	return render.Subquery{Table: sq.table, Columns: sq.columns, Where: sq.where, Order: sq.order, Limit: sq.limit, Offset: sq.offset}, nil
}

// rowFeed is a orm.Row that hands back previously-scanned driver values in
// order instead of reading from a database cursor. scanJoinRow/
// scanLeftJoinRow scan the joined SELECT's raw values into a flat holder
// slice with ONE rows.Scan call, then feed each entity's own Scan method
// the holder values for its half of the row via rowFeed -- so an entity's
// Scan performs its real value conversion (e.g. the codegen'd RFC3339Nano
// timestamp parse) against the ACTUAL scanned values. The older
// collectRow approach could not do that: it only recorded destination
// pointers, so a Scan that post-processed what it scanned (a timestamp
// column) ran against still-empty values and failed.
type rowFeed struct {
	vals []any
	idx  int
}

func (r *rowFeed) Scan(dest ...any) error {
	for i, d := range dest {
		if r.idx >= len(r.vals) {
			return fmt.Errorf("orm: join scan: row feed exhausted after %d values", i)
		}

		v := r.vals[r.idx]
		r.idx++

		if err := assignAny(d, v); err != nil {
			return fmt.Errorf("orm: join scan: %w", err)
		}
	}

	return nil
}

// scanJoinRow scans one joined row into a Row2[A, B] via exactly one
// underlying rows.Scan call. aCols/bCols are the column counts of A's and
// B's tables, in the order the joined SELECT projects them.
func scanJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](rows db.Rows, aCols, bCols int) (Row2[A, B], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: join scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: join scan: %w", err)
	}

	var b B

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: join scan: %w", err)
	}

	return Row2[A, B]{A: a, B: b}, nil
}

// scanLeftJoinRow scans one LEFT-JOINed row into a Row2[A, Option[B]] via
// exactly one underlying rows.Scan call. A's half is handed to A's own
// Scan through a rowFeed exactly like scanJoinRow; B's half is scanned into
// *any holders instead (database/sql always accepts a NULL into an *any
// destination, storing a nil), the holders are checked for all-nil (==
// unmatched right side), and only when at least one is non-nil is B's own
// Scan fed those values -- so B's Scan, including any post-processing such
// as a timestamp parse, runs against real values, and an unmatched row is
// unambiguously None rather than a zero-valued B.
func scanLeftJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](rows db.Rows, aCols, bCols int) (Row2[A, Option[B]], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left join scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left join scan: %w", err)
	}

	allNull := allNil(holders[aCols:])

	if allNull {
		return Row2[A, Option[B]]{A: a, B: None[B]()}, nil
	}

	var b B

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left join scan: %w", err)
	}

	return Row2[A, Option[B]]{A: a, B: Some(b)}, nil
}

// scanRightJoinRow scans one RIGHT-JOINed row into a Row2[Option[A], B] via
// exactly one underlying rows.Scan call. B's half is handed to B's own Scan
// through a rowFeed exactly like scanJoinRow (a RIGHT JOIN preserves every B
// row, so B can never be all-NULL); A's half is scanned into *any holders,
// checked for all-nil (== unmatched right side), and only when at least one
// is non-nil is A's own Scan fed those values -- so an unmatched row is
// unambiguously None rather than a zero-valued A.
func scanRightJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](rows db.Rows, aCols, bCols int) (Row2[Option[A], B], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[Option[A], B]{}, fmt.Errorf("orm: right join scan: %w", err)
	}

	var b B

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
		return Row2[Option[A], B]{}, fmt.Errorf("orm: right join scan: %w", err)
	}

	if allNil(holders[:aCols]) {
		return Row2[Option[A], B]{A: None[A](), B: b}, nil
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row2[Option[A], B]{}, fmt.Errorf("orm: right join scan: %w", err)
	}

	return Row2[Option[A], B]{A: Some(a), B: b}, nil
}

// scanFullJoinRow scans one FULL-JOINed row into a Row2[Option[A], Option[B]]
// via exactly one underlying rows.Scan call. BOTH halves are scanned into
// *any holders and independently checked for all-nil -- a FULL JOIN can put
// NULLs on either side -- so an unmatched left row comes back B = None and
// an unmatched right row A = None, each side's Option wrap unambiguous.
func scanFullJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](rows db.Rows, aCols, bCols int) (Row2[Option[A], Option[B]], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: full join scan: %w", err)
	}

	row := Row2[Option[A], Option[B]]{}

	if allNil(holders[:aCols]) {
		row.A = None[A]()
	} else {
		var a A

		if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
			return Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: full join scan: %w", err)
		}

		row.A = Some(a)
	}

	if allNil(holders[aCols:]) {
		row.B = None[B]()
	} else {
		var b B

		if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
			return Row2[Option[A], Option[B]]{}, fmt.Errorf("orm: full join scan: %w", err)
		}

		row.B = Some(b)
	}

	return row, nil
}

// scanJoinRow3 scans one three-table joined row into a Row3[A, B, C] via
// exactly one underlying rows.Scan call. All three halves are scanned
// directly through their own Scan methods -- scanJoinRow generalized to
// three tables -- which is only sound for an INNER (or caller-proven
// non-null) three-way join (see Join3's doc comment for why).
func scanJoinRow3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](rows db.Rows, aCols, bCols, cCols int) (Row3[A, B, C], error) {
	holders, dests := joinHolders(aCols + bCols + cCols)

	if err := rows.Scan(dests...); err != nil {
		return Row3[A, B, C]{}, fmt.Errorf("orm: join3 scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row3[A, B, C]{}, fmt.Errorf("orm: join3 scan: %w", err)
	}

	var b B

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols : aCols+bCols]}); err != nil {
		return Row3[A, B, C]{}, fmt.Errorf("orm: join3 scan: %w", err)
	}

	var c C

	if err := PC(&c).Scan(&rowFeed{vals: holders[aCols+bCols:]}); err != nil {
		return Row3[A, B, C]{}, fmt.Errorf("orm: join3 scan: %w", err)
	}

	return Row3[A, B, C]{A: a, B: b, C: c}, nil
}

// scanLeftJoinRow3 scans one three-table LEFT-JOINed row into a
// Row3[A, Option[B], Option[C]] via exactly one underlying rows.Scan call.
// A's half is handed to A's own Scan through a rowFeed exactly like
// scanJoinRow3; B's and C's halves are each scanned through the shared
// scanOptional helper, so an unmatched side becomes an unambiguous None and
// no side's Scan runs against a fake zero value. The SQL LEFT-chain
// guarantees C Some implies B Some, so no cross-side reconciliation is
// needed here.
func scanLeftJoinRow3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](rows db.Rows, aCols, bCols, cCols int) (Row3[A, Option[B], Option[C]], error) {
	holders, dests := joinHolders(aCols + bCols + cCols)

	if err := rows.Scan(dests...); err != nil {
		return Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: left join3 scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: left join3 scan: %w", err)
	}

	b, err := scanOptional[B, PB](holders[aCols : aCols+bCols])
	if err != nil {
		return Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: left join3 scan: %w", err)
	}

	c, err := scanOptional[C, PC](holders[aCols+bCols:])
	if err != nil {
		return Row3[A, Option[B], Option[C]]{}, fmt.Errorf("orm: left join3 scan: %w", err)
	}

	return Row3[A, Option[B], Option[C]]{A: a, B: b, C: c}, nil
}

// scanRightJoinRow3 scans one three-table RIGHT-JOINed row into a
// Row3[Option[A], Option[B], C] via exactly one underlying rows.Scan call.
// C is always present (a RIGHT JOIN preserves every right-side row), so it
// is handed to C's own Scan through a rowFeed exactly like scanJoinRow3; A's
// and B's halves are each scanned through the shared scanOptional helper, so
// an unmatched side becomes an unambiguous None. The SQL RIGHT-chain
// guarantees A Some implies B Some.
func scanRightJoinRow3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](rows db.Rows, aCols, bCols, cCols int) (Row3[Option[A], Option[B], C], error) {
	holders, dests := joinHolders(aCols + bCols + cCols)

	if err := rows.Scan(dests...); err != nil {
		return Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: right join3 scan: %w", err)
	}

	var c C

	if err := PC(&c).Scan(&rowFeed{vals: holders[aCols+bCols:]}); err != nil {
		return Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: right join3 scan: %w", err)
	}

	a, err := scanOptional[A, PA](holders[:aCols])
	if err != nil {
		return Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: right join3 scan: %w", err)
	}

	b, err := scanOptional[B, PB](holders[aCols : aCols+bCols])
	if err != nil {
		return Row3[Option[A], Option[B], C]{}, fmt.Errorf("orm: right join3 scan: %w", err)
	}

	return Row3[Option[A], Option[B], C]{A: a, B: b, C: c}, nil
}

// scanFullJoinRow3 scans one three-table FULL-JOINed row into a
// Row3[Option[A], Option[B], Option[C]] via exactly one underlying rows.Scan
// call. A FULL JOIN can put NULLs on any side, so all three halves are
// scanned through the shared scanOptional helper -- each side's Option wrap
// is unambiguous, never a zero struct.
func scanFullJoinRow3[
	A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C],
](rows db.Rows, aCols, bCols, cCols int) (Row3[Option[A], Option[B], Option[C]], error) {
	holders, dests := joinHolders(aCols + bCols + cCols)

	if err := rows.Scan(dests...); err != nil {
		return Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: full join3 scan: %w", err)
	}

	a, err := scanOptional[A, PA](holders[:aCols])
	if err != nil {
		return Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: full join3 scan: %w", err)
	}

	b, err := scanOptional[B, PB](holders[aCols : aCols+bCols])
	if err != nil {
		return Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: full join3 scan: %w", err)
	}

	c, err := scanOptional[C, PC](holders[aCols+bCols:])
	if err != nil {
		return Row3[Option[A], Option[B], Option[C]]{}, fmt.Errorf("orm: full join3 scan: %w", err)
	}

	return Row3[Option[A], Option[B], Option[C]]{A: a, B: b, C: c}, nil
}

// allNil reports whether every holder in holders holds a nil -- i.e. the
// scanned half of an outer-join row came back all-NULL (an unmatched side).
func allNil(holders []any) bool {
	for _, h := range holders {
		if h != nil {
			return false
		}
	}

	return true
}

// joinHolders returns a slice of *any value holders and a parallel slice of
// destination pointers covering them, sized for a joined row of n columns.
func joinHolders(n int) (holders, dests []any) {
	holders = make([]any, n)
	dests = make([]any, n)

	for i := range holders {
		dests[i] = &holders[i]
	}

	return holders, dests
}

// assignAny converts src (a raw driver-ish value, or nil) into dest, a
// pointer previously handed to a codegen'd entity's Scan method. If dest
// itself implements the same single-argument Scan(any) error shape
// orm.Option[T] does, that Scan is called directly -- Option[T]'s own Scan
// already handles both a real value and NULL correctly. Otherwise dest is
// one of the small closed set of concrete pointer types the schema code generator's Scan
// codegen ever produces (see orm/option.go's convertScan, which this
// mirrors for the same reason: no reflection, a clear error for anything
// outside that set).
func assignAny(dest any, src any) error {
	if scanner, ok := dest.(interface{ Scan(src any) error }); ok {
		return scanner.Scan(src)
	}

	switch d := dest.(type) {
	case *any:
		// A table-valued JSON source's dynamically typed column (SQLite
		// json_each/json_tree's `value`/`key`/`atom`/`root`, or Postgres's
		// jsonb `value`) scans into a bare any: the driver value (or nil for
		// NULL) is stored as-is, exactly the shape a caller's json_each row
		// struct needs. This is the one dest type outside the closed
		// codegen set, and it is added for the TVF scan path only.
		*d = src
	case *string:
		v, err := scanString(src)
		if err != nil {
			return err
		}

		*d = v
	case *[]byte:
		v, err := scanBytes(src)
		if err != nil {
			return err
		}

		*d = v
	case *int64:
		v, err := scanInt64(src)
		if err != nil {
			return err
		}

		*d = v
	case *int32:
		v, err := scanInt64(src)
		if err != nil {
			return err
		}

		*d = int32(v) //nolint:gosec // narrowing matches the driver-returned column width, mirrors orm.Option's own convertScan
	case *float64:
		v, err := scanFloat64(src)
		if err != nil {
			return err
		}

		*d = v
	case *float32:
		v, err := scanFloat64(src)
		if err != nil {
			return err
		}

		*d = float32(v)
	case *bool:
		v, err := scanBool(src)
		if err != nil {
			return err
		}

		*d = v
	default:
		return fmt.Errorf("orm: assignAny: unsupported destination type %T", dest)
	}

	return nil
}
