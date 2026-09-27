package orm

import (
	"context"
	"fmt"
	"iter"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// Mixed-kind three-table joins.
//
// #409 shipped the four homogeneous three-table builders -- Join3
// (INNER/INNER), LeftJoin3 (LEFT/LEFT), RightJoin3 (RIGHT/RIGHT) and
// FullJoin3 (FULL/FULL) -- and #413 added the two INNER/LEFT mixed chains:
//
// - InnerLeftJoin3: A INNER JOIN B LEFT JOIN C -> Row3[A, B, Option[C]]
// - LeftInnerJoin3: A LEFT JOIN (B INNER JOIN C) -> Row3[A, Option[B], Option[C]]
//
// Together with Join3 and LeftJoin3 those cover every combination of the
// two universal join kinds. This file's MixedJoin3 completes the picture:
// it renders the flat, left-associative chain
//
//	A <abKind> JOIN B ON relAB <bcKind> JOIN C ON relBC
//
// for ANY (abKind, bcKind) pair -- all 16 combinations uniformly -- with a
// fully-optional Row3[Option[A], Option[B], Option[C]] result. Go cannot
// compute a per-combination nullability type at the type level, and a
// fully-optional Row3 is ALWAYS correct: for any chain every side may be
// absent in some row, and presenting a guaranteed-present side as Some is
// still correct. The dedicated builders above (and Join3/LeftJoin3/
// RightJoin3/FullJoin3) keep their precise per-side types for the common
// chains; reach for MixedJoin3 only when the kind pair is dynamic or not
// covered by a dedicated builder.
//
// The flat render is SQL-standard left associativity -- (A abKind B) bcKind
// C -- and is valid on every dialect that supports the individual kinds.
// This is deliberately NOT the parenthesized composite LeftInnerJoin3 uses
// for "A LEFT JOIN (B INNER JOIN C)": a chain whose second hop is
// non-preserving on B (e.g. LEFT then INNER) flat-renders as
// (A LEFT JOIN B) INNER JOIN C and can drop A rows the first LEFT join was
// asked to preserve. Callers who need those composite semantics must use
// LeftInnerJoin3; MixedJoin3 documents and follows the standard flat chain.

// InnerLeftJoin3 is the mixed-kind three-table builder for
// "A INNER JOIN B ON ... LEFT JOIN C ON ...": A and B are inner-joined (so
// every result row carries a real A and B), then C is left-joined (so C is
// optional). Its All/Stream return Row3[A, B, Option[C]] -- A and B scanned
// directly through their own Scan methods, C wrapped whole in Option so an
// unmatched C is unambiguously None, never a same-shaped zero struct
// (mirroring LeftJoin2's whole-struct wrap).
//
// This chain renders flat (SelectJoin3Mixed): because the first hop is
// INNER, the A/B row set is already fixed before C is left-joined, so no
// outer-join semantics are lost. Both INNER and LEFT are universal SQL, so
// InnerLeftJoin3 is never capability-gated.
//
// For the opposite INNER/LEFT ordering -- A LEFT JOIN (B INNER JOIN C) --
// use LeftInnerJoin3, which needs the parenthesized composite render to
// keep A rows whose B is absent.
type InnerLeftJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
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

// InnerLeftJoinOn3 starts an InnerLeftJoin3 from left through relAB to B,
// then relBC to C -- the mixed-chain counterpart of LeftJoinOn3.
func InnerLeftJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	return InnerLeftJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC}
}

// Where combines p into j's A-side filter with AND.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies to the optional C side here too.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) Limit(n int) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) Offset(n int) InnerLeftJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// render turns j's accumulated state into SQL text plus positional args,
// rendering the flat mixed chain "A INNER JOIN B ON ... LEFT JOIN C ON ...".
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3Mixed[A, PA, B, PB, C, PC](d, InnerJoin, LeftJoin, false, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[A, B, Option[C]], scanned via ONE rows.Scan call per row (see
// scanInnerLeftJoinRow3) -- one round trip total.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[A, B, Option[C]], error) {
	return gatherJoin3(ctx, exec, "InnerLeftJoin3", j.render,
		scanInnerLeftJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j InnerLeftJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[A, B, Option[C]], error] {
	return streamJoin3(ctx, exec, "InnerLeftJoin3", j.render,
		scanInnerLeftJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}

// LeftInnerJoin3 is the mixed-kind three-table builder for
// "A LEFT JOIN (B INNER JOIN C)": A is always present, and the parenthesized
// B/C composite is left-joined to it.
//
// The B/C pair is joined INNER first and rendered as one parenthesized
// derived table (SelectJoin3Nested), so it is all-or-nothing: a row either
// has both a real B and a real C, or neither. Its All/Stream therefore
// return Row3[A, Option[B], Option[C]] with the equivalences B Some <=> C
// Some -- in particular the documented invariant C Some => B Some, and its
// contrapositive B None => C None.
//
// Rendering this chain flat as "A LEFT JOIN B ON ... INNER JOIN C ON ..."
// would be WRONG: SQL parses it left-associatively as
// (A LEFT JOIN B) INNER JOIN C, and because the C join keys on B, every A
// row whose B is absent fails the C join and is silently dropped -- turning
// the LEFT join into an inner one. The parenthesized composite render is
// what keeps A rows. This is why LeftInnerJoin3 must NOT be modeled as a
// flat mixed chain (contrast InnerLeftJoin3).
//
// Both INNER and LEFT are universal SQL, so LeftInnerJoin3 is never
// capability-gated.
type LeftInnerJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
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

// LeftInnerJoinOn3 starts a LeftInnerJoin3 from left through relAB to B,
// then relBC to C -- the mixed-chain counterpart of LeftJoinOn3, rendered
// as A LEFT JOIN (B INNER JOIN C ON relBC) ON relAB.
func LeftInnerJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	return LeftInnerJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC}
}

// Where combines p into j's A-side filter with AND. Because A is the
// preserved side of the outer join, a Where on A never turns the join back
// into an inner one.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically to the optional composite here.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies identically to the optional composite here.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) Limit(n int) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) Offset(n int) LeftInnerJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// render turns j's accumulated state into SQL text plus positional args,
// rendering the parenthesized composite "A LEFT JOIN (B INNER JOIN C ON
// ...) ON ...".
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3Mixed[A, PA, B, PB, C, PC](d, LeftJoin, InnerJoin, true, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[A, Option[B], Option[C]], scanned via ONE rows.Scan call per row (see
// scanLeftInnerJoinRow3) -- one round trip total.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[A, Option[B], Option[C]], error) {
	// A is direct, B/C optional -- exactly scanLeftJoinRow3's shape, reused
	// rather than duplicated: only the rendered SQL differs between a
	// flat LEFT-LEFT chain and this nested LEFT-INNER one.
	return gatherJoin3(ctx, exec, "LeftInnerJoin3", j.render,
		scanLeftJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
func (j LeftInnerJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[A, Option[B], Option[C]], error] {
	return streamJoin3(ctx, exec, "LeftInnerJoin3", j.render,
		scanLeftJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}

// scanOptional scans one side of an outer-joined row into Option[T] via a
// holder slice already populated by the caller's single rows.Scan: all-nil
// means the side did not match, so the result is None and T's own Scan is
// never run against a fake zero value; otherwise T's Scan is fed the real
// values through a rowFeed. Every optional-side scanner below shares this
// one helper, so the "all-nil vs real value" decision cannot drift between
// variants.
func scanOptional[T any, PT ptrScanner[T]](holders []any) (Option[T], error) {
	if allNil(holders) {
		return None[T](), nil
	}

	var v T

	if err := PT(&v).Scan(&rowFeed{vals: holders}); err != nil {
		return Option[T]{}, err
	}

	return Some(v), nil
}

// scanInnerLeftJoinRow3 scans one "A INNER JOIN B LEFT JOIN C" row into a
// Row3[A, B, Option[C]] via exactly one underlying rows.Scan call. A and B
// are always present (the inner join guarantees both), so each is handed to
// its own Scan through a rowFeed; C is scanned through scanOptional so an
// unmatched C becomes an unambiguous None.
func scanInnerLeftJoinRow3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](rows db.Rows, aCols, bCols, cCols int) (Row3[A, B, Option[C]], error) {
	holders, dests := joinHolders(aCols + bCols + cCols)

	if err := rows.Scan(dests...); err != nil {
		return Row3[A, B, Option[C]]{}, fmt.Errorf("orm: inner-left join3 scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row3[A, B, Option[C]]{}, fmt.Errorf("orm: inner-left join3 scan: %w", err)
	}

	var b B

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols : aCols+bCols]}); err != nil {
		return Row3[A, B, Option[C]]{}, fmt.Errorf("orm: inner-left join3 scan: %w", err)
	}

	c, err := scanOptional[C, PC](holders[aCols+bCols:])
	if err != nil {
		return Row3[A, B, Option[C]]{}, fmt.Errorf("orm: inner-left join3 scan: %w", err)
	}

	return Row3[A, B, Option[C]]{A: a, B: b, C: c}, nil
}

// gatherJoin3 runs a rendered three-table join and scans every row with
// scan, issuing exactly one query and one rows.Scan per row. The mixed
// builders whose All return the same Row3 shape share this body; label keeps
// each caller's "orm: <Type>.All" error tag without duplicating the logic.
func gatherJoin3[Res any](
	ctx context.Context,
	exec db.DB,
	label string,
	render func(d dialect.Dialect) (string, []any, error),
	scan func(rows db.Rows, aCols, bCols, cCols int) (Res, error),
	aCols, bCols, cCols int,
) ([]Res, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: %s.All: %w", label, err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: %s.All: %w", label, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Res

	for rows.Next() {
		row, err := scan(rows, aCols, bCols, cCols)
		if err != nil {
			return nil, fmt.Errorf("orm: %s.All: scan: %w", label, err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: %s.All: %w", label, err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: %s.All: %w", label, err)
	}

	return out, nil
}

// streamJoin3 is gatherJoin3's streaming counterpart: same one-query,
// one-scan-per-row contract, yielded incrementally. The range-over-func
// cleanup guarantee is Join2.Stream's (closing rows on early break).
func streamJoin3[Res any](
	ctx context.Context,
	exec db.DB,
	label string,
	render func(d dialect.Dialect) (string, []any, error),
	scan func(rows db.Rows, aCols, bCols, cCols int) (Res, error),
	aCols, bCols, cCols int,
) iter.Seq2[Res, error] {
	return func(yield func(Res, error) bool) {
		var zero Res

		d, err := resolveDialect(exec)
		if err != nil {
			yield(zero, err)

			return
		}

		query, args, err := render(d)
		if err != nil {
			yield(zero, fmt.Errorf("orm: %s.Stream: %w", label, err))

			return
		}

		rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
		if err != nil {
			yield(zero, fmt.Errorf("orm: %s.Stream: %w", label, err))

			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			row, err := scan(rows, aCols, bCols, cCols)
			if err != nil {
				yield(zero, fmt.Errorf("orm: %s.Stream: scan: %w", label, err))

				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(zero, fmt.Errorf("orm: %s.Stream: %w", label, err))

			return
		}

		if err := rows.Close(); err != nil {
			yield(zero, fmt.Errorf("orm: %s.Stream: %w", label, err))
		}
	}
}

// MixedJoin3 is the generic mixed-kind three-table join builder: it renders
// the flat, left-associative chain
//
//	A <abKind> JOIN B ON relAB <bcKind> JOIN C ON relBC
//
// for ANY pair of join kinds -- all 16 (abKind, bcKind) combinations render
// through the same type and methods -- and whose All/Stream return a
// fully-optional Row3[Option[A], Option[B], Option[C]].
//
// # Why a fully-optional result for every combination
//
// Go's type system cannot compute a per-combination nullability type from
// the runtime abKind/bcKind values (the same limitation Join2's doc comment
// describes for its two-table case), so one result type has to serve all 16
// combinations. A fully-optional Row3 is always correct: in any chain every
// side may be absent in some row, and presenting a guaranteed-present side
// as Some is still correct -- it only loses the "this Option can never be
// None" refinement, never soundness. Unmatched sides are unambiguously None
// (via the shared scanOptional helper), never a same-shaped zero struct.
//
// Callers who know their chain's exact kinds should prefer the dedicated
// builders, which keep precise per-side types: Join3 (Row3[A, B, C]),
// LeftJoin3 (Row3[A, Option[B], Option[C]]), RightJoin3
// (Row3[Option[A], Option[B], C]), FullJoin3 (all Option),
// InnerLeftJoin3 (Row3[A, B, Option[C]]) and LeftInnerJoin3
// (Row3[A, Option[B], Option[C]]). MixedJoin3 is the escape hatch for a
// kind pair selected at runtime or otherwise not covered above.
//
// # SQL semantics: flat, left-associative
//
// The chain renders flat and left-associative, exactly as SQL parses it:
// (A abKind B) bcKind C. This is the first-class standard shape and is valid
// on every dialect supporting the individual kinds. It is deliberately NOT
// the parenthesized composite that LeftInnerJoin3 uses for "A LEFT JOIN (B
// INNER JOIN C)": a chain whose second hop is non-preserving on B (for
// example LEFT then INNER) is evaluated as (A LEFT JOIN B) INNER JOIN C and
// can drop A rows the first LEFT join was asked to preserve. Callers who need
// the composite semantics must use LeftInnerJoin3; MixedJoin3 follows
// standard SQL flat associativity and does not invent parenthesized forms.
//
// # Capability gating
//
// requireJoin gates EACH hop with its own kind before any SQL is issued: a
// RightJoin hop requires dialect.JoinCapabilities.SupportsRightJoin and a
// FullJoin hop requires SupportsFullJoin. On a dialect lacking the
// capability, All/Stream return the typed
// dialect.ErrUnsupportedByDialect before executing SQL -- Postgres supports
// both, MySQL supports RIGHT only, and the default SQLite dialect supports
// neither (INNER/LEFT-only combinations still run there).
type MixedJoin3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]] struct {
	left   Query[A, PA]
	relAB  Relation[A, B]
	relBC  Relation[B, C]
	abKind JoinType
	bcKind JoinType
	whereB Predicate[B]
	whereC Predicate[C]
	order  []OrderTerm[A]
	orderB []OrderTerm[B]
	orderC []OrderTerm[C]
	limit  int
	offset int
}

// MixedJoinOn3 starts a MixedJoin3 from left through relAB to B, then relBC
// to C. abKind is the A/B hop's join keyword and bcKind the B/C hop's; they
// may differ (and may each be INNER/LEFT/RIGHT/FULL). See MixedJoin3's doc
// comment for the flat left-associative semantics and the fully-optional
// result.
func MixedJoinOn3[A any, PA ptrScanner[A], B any, PB ptrScanner[B], C any, PC ptrScanner[C]](
	left Query[A, PA],
	relAB Relation[A, B],
	relBC Relation[B, C],
	abKind JoinType,
	bcKind JoinType,
) MixedJoin3[A, PA, B, PB, C, PC] {
	return MixedJoin3[A, PA, B, PB, C, PC]{left: left, relAB: relAB, relBC: relBC, abKind: abKind, bcKind: bcKind}
}

// Where combines p into j's A-side filter with AND.
func (j MixedJoin3[A, PA, B, PB, C, PC]) Where(p Predicate[A]) MixedJoin3[A, PA, B, PB, C, PC] {
	j.left = j.left.Where(p)

	return j
}

// WhereRight combines p into j's B-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies here too (a filter on a side that a hop can NULL out behaves
// like an inner join for the rows it excludes).
func (j MixedJoin3[A, PA, B, PB, C, PC]) WhereRight(p Predicate[B]) MixedJoin3[A, PA, B, PB, C, PC] {
	if j.whereB.IsSet() {
		j.whereB = And(j.whereB, p)
	} else {
		j.whereB = p
	}

	return j
}

// WhereC combines p into j's C-side filter with AND; see
// LeftJoin2.WhereRight's doc comment for the WHERE-on-an-outer-join caveat
// that applies here too.
func (j MixedJoin3[A, PA, B, PB, C, PC]) WhereC(p Predicate[C]) MixedJoin3[A, PA, B, PB, C, PC] {
	if j.whereC.IsSet() {
		j.whereC = And(j.whereC, p)
	} else {
		j.whereC = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the A table only.
func (j MixedJoin3[A, PA, B, PB, C, PC]) OrderBy(terms ...OrderTerm[A]) MixedJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends ORDER BY terms scoped to the B table only.
func (j MixedJoin3[A, PA, B, PB, C, PC]) OrderByRight(terms ...OrderTerm[B]) MixedJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[B], 0, len(j.orderB)+len(terms))
	next = append(next, j.orderB...)
	next = append(next, terms...)
	j.orderB = next

	return j
}

// OrderByC appends ORDER BY terms scoped to the C table only.
func (j MixedJoin3[A, PA, B, PB, C, PC]) OrderByC(terms ...OrderTerm[C]) MixedJoin3[A, PA, B, PB, C, PC] {
	next := make([]OrderTerm[C], 0, len(j.orderC)+len(terms))
	next = append(next, j.orderC...)
	next = append(next, terms...)
	j.orderC = next

	return j
}

// Limit sets j's row limit.
func (j MixedJoin3[A, PA, B, PB, C, PC]) Limit(n int) MixedJoin3[A, PA, B, PB, C, PC] {
	j.limit = n

	return j
}

// Offset sets j's row offset.
func (j MixedJoin3[A, PA, B, PB, C, PC]) Offset(n int) MixedJoin3[A, PA, B, PB, C, PC] {
	j.offset = n

	return j
}

// render turns j's accumulated state into SQL text plus positional args,
// rendering the flat, left-associative mixed chain and gating EACH hop
// through renderJoin3Mixed's requireJoin calls.
func (j MixedJoin3[A, PA, B, PB, C, PC]) render(d dialect.Dialect) (string, []any, error) {
	return renderJoin3Mixed[A, PA, B, PB, C, PC](d, j.abKind, j.bcKind, false, j.left, j.relAB, j.relBC, j.whereB, j.whereC, j.order, j.orderB, j.orderC, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row3[Option[A], Option[B], Option[C]], scanned via ONE rows.Scan call per
// row (see scanFullJoinRow3, whose optional-every-side shape is exactly what
// the generic builder needs) -- one round trip total. A RIGHT/FULL hop on a
// dialect without the capability returns the typed
// dialect.ErrUnsupportedByDialect before any SQL is issued.
func (j MixedJoin3[A, PA, B, PB, C, PC]) All(ctx context.Context, exec db.DB) ([]Row3[Option[A], Option[B], Option[C]], error) {
	return gatherJoin3(ctx, exec, "MixedJoin3", j.render,
		scanFullJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}

// Stream runs j against exec and yields every matching row one at a time.
// See Join2.Stream's doc comment for the range-over-func cleanup guarantee.
// Like All, a missing RIGHT/FULL capability is the typed
// dialect.ErrUnsupportedByDialect, yielded before any SQL is issued.
func (j MixedJoin3[A, PA, B, PB, C, PC]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row3[Option[A], Option[B], Option[C]], error] {
	return streamJoin3(ctx, exec, "MixedJoin3", j.render,
		scanFullJoinRow3[A, PA, B, PB, C, PC],
		len(j.left.table.Columns()), len(j.relAB.childTable.Columns()), len(j.relBC.childTable.Columns()))
}
