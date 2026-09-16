package orm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// LockMode identifies the row-level lock a Query's SELECT acquires. Its
// values mirror render.LockMode's (same underlying int representation), so
// the renderer gets a plain type conversion at the call site.
type LockMode int

// Supported row-lock modes. LockNone is the zero value and renders no lock.
const (
	LockNone LockMode = iota
	LockForUpdate
	LockForShare
	// LockForNoKeyUpdate and LockForKeyShare are Postgres-only weaker lock
	// strengths (`FOR NO KEY UPDATE`, `FOR KEY SHARE`); other dialects
	// reject them with a typed dialect.ErrUnsupportedByDialect.
	LockForNoKeyUpdate
	LockForKeyShare
)

// Typed errors for invalid locking usage. They are dialect-independent
// caller bugs testable with errors.Is; a lock mode the resolved dialect
// lacks is instead reported as dialect.ErrUnsupportedByDialect.
var (
	// ErrLockingWithDistinct is returned when DISTINCT is combined with
	// FOR UPDATE/FOR SHARE. Standard SQL (and Postgres and SQLite) forbid
	// locking rows in a DISTINCT result, so orm fails closed rather than
	// emitting invalid SQL.
	ErrLockingWithDistinct = errors.New("orm: DISTINCT cannot be combined with a row lock (FOR UPDATE/FOR SHARE)")
	// ErrLockingRequiresLockMode is returned when NOWAIT or SKIP LOCKED is
	// used without a preceding FOR UPDATE/FOR SHARE.
	ErrLockingRequiresLockMode = errors.New("orm: NOWAIT and SKIP LOCKED require FOR UPDATE or FOR SHARE")
	// ErrLockingNotSelect is returned when a row lock is requested on
	// Count/Exists, which render an aggregate and cannot hold a row lock.
	ErrLockingNotSelect = errors.New("orm: row locking is only supported on SELECT queries, not Count/Exists")
)

// Query[T, PT] uses two type parameters: T is the entity's plain struct
// type (User) -- matching Table[T]/Column[T,V]'s existing T -- and PT is
// constrained to `*T` plus Scanner, letting Query construct a new,
// addressable T and scan into it through PT's method set with zero
// reflection (the "curiously recurring generic pattern": Go's constraint
// type inference resolves PT from T at call sites, so ordinary callers
// write `orm.From(Users)`, never `orm.From[User, *User](Users)`).
// All/First return PT (i.e. *User), matching the pointer convention.
type ptrScanner[T any] interface {
	*T
	Scanner
}

// NullsOrder selects where SQL NULLs sort within an ORDER BY term. The zero
// value NullsDefault leaves the position to the dialect's default; NullsFirst
// and NullsLast force NULLs to the front/back. The modifier is gated by
// dialect.NullsOrderDialect at render time: Postgres and SQLite >= 3.30.0
// support it; other dialects do not (see OrderTerm.NullsFirst for the
// documented CASE-expression workaround).
type NullsOrder int

// Supported NULLS-ordering positions. NullsDefault renders no NULLS suffix.
const (
	NullsDefault NullsOrder = iota
	NullsFirst
	NullsLast
)

// OrderTerm is one column of an ORDER BY clause, plus its direction.
// Column is an AnyColumn[T] -- an erased reference built only from a
// schema-derived Column[T,V] via Column.Col() -- never a raw string, so a
// caller cannot inject SQL structure through OrderBy. FTS is non-nil only
// for an order term built by NewFTSOrderTerm: it orders by an FTS ranking
// expression (Postgres ts_rank) instead of a plain quoted column -- see
// render/fts.go. SQLite's FTS5 rank ordering needs no expression (its
// hidden rank column is a plain OrderTerm). Func is non-nil only for a term
// built by Expr.Asc/Desc: it orders by a scalar expression tree, e.g.
// ORDER BY COALESCE("bio", ?) -- see orm/expr.go. Nulls is the optional
// `NULLS FIRST`/`NULLS LAST` suffix set by NullsFirst/NullsLast.
type OrderTerm[T any] struct {
	Column AnyColumn[T]
	Desc   bool
	Nulls  NullsOrder
	FTS    *FTSExpr
	Func   *FuncExpr
}

// NullsFirst returns a copy of t that sorts NULLs before non-NULL values:
// `"col" ASC NULLS FIRST` (or DESC, per t's direction). It leaves the
// receiver unmodified. The modifier is native on Postgres and SQLite >=
// 3.30.0; on a dialect that does not support it, execution returns the
// typed dialect.ErrUnsupportedByDialect rather than silently dropping it.
//
// Dialects without NULLS FIRST/LAST can get the same ordering portably by
// ordering by a CASE that ranks NULLs and then by the column:
//
//	Case[widget, int]().When(c.IsNull(), 1).Else(0).Asc()
//
// for NULLS LAST (the example's `1`/`0` swapped for NULLS FIRST).
func (t OrderTerm[T]) NullsFirst() OrderTerm[T] {
	t.Nulls = NullsFirst

	return t
}

// NullsLast returns a copy of t that sorts NULLs after non-NULL values:
// `"col" ASC NULLS LAST` (or DESC, per t's direction). It leaves the
// receiver unmodified. See NullsFirst for the dialect gate and the portable
// CASE-expression alternative.
func (t OrderTerm[T]) NullsLast() OrderTerm[T] {
	t.Nulls = NullsLast

	return t
}

// Query is an immutable, value-type SQL SELECT builder over entity
// T. Every chain method (Where/OrderBy/Limit/Offset) returns a NEW Query
// value rather than mutating the receiver, and every method that appends
// to a slice field copies into a fresh backing array first, so branching a
// base Query across goroutines/call sites is safe without a persistent
// data structure.
type Query[T any, PT ptrScanner[T]] struct {
	table       Table[T]
	columns     []string
	where       Predicate[T]
	order       []OrderTerm[T]
	limit       int
	offset      int
	distinct    bool
	distinctOn  []string
	lock        LockMode
	lockOf      []string
	tablesample *Tablesample
	nowait      bool
	skipLocked  bool
}

// Tablesample is the Postgres `TABLESAMPLE <method> (<percentage>)` FROM
// clause: Method is one of SYSTEM or BERNOULLI and Arg the sampling
// percentage in [0, 100].
type Tablesample struct {
	Method string
	Arg    float64
}

// From builds a Query over t with no filter, ordering, limit or offset
// set. The dialect to render against is resolved later, at All/First/
// Count/Exists call time, from the db.DB argument's own Dialect() name --
// so a Query value itself never needs to carry a resolved Dialect.
func From[T any, PT ptrScanner[T]](t Table[T]) Query[T, PT] {
	return Query[T, PT]{table: t}
}

// Columns projects q onto cols: the returned Query SELECTs exactly those
// columns (in order) instead of the entity table's full column list, and
// the codegen'd Scan method then reads each row POSITIONALLY, so a
// projection must match Scan's expected column order exactly in any
// standalone All/Stream use (in practice that means projecting the full
// entity; the projection's real use is rendering subquery operands, whose
// rows are never scanned). Every col must be a codegen-derived
// Column/NullableColumn erased via Col() -- never a raw string -- so a
// caller cannot inject SQL structure through Columns. It is a
// copy-on-write method like Where/OrderBy: the receiver is left
// unmodified.
//
// Projection is what makes a Query usable as a subquery operand: an
// In/NotIn-subquery or scalar comparison renders the inner query's
// projection, which must contain exactly ONE column.
func (q Query[T, PT]) Columns(cols ...AnyColumn[T]) Query[T, PT] {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name()
	}

	q.columns = names

	return q
}

// selectColumns returns the columns q SELECTs: the Columns(...) projection
// when one is set, else the entity table's full column list. The result is
// a fresh slice -- never the underlying array -- so callers (the renderer,
// toSubquery) cannot mutate Query state through it.
func (q Query[T, PT]) selectColumns() []string {
	if len(q.columns) > 0 {
		cp := make([]string, len(q.columns))
		copy(cp, q.columns)

		return cp
	}

	return q.table.Columns()
}

// Where combines p into q's existing filter with AND: the receiver is left
// unmodified, and the returned Query's where is q.where AND p when q
// already had one set, or just p otherwise. See ExampleQuery_Where for a
// runnable example.
func (q Query[T, PT]) Where(p Predicate[T]) Query[T, PT] {
	if q.where.IsSet() {
		q.where = And(q.where, p)
	} else {
		q.where = p
	}

	return q
}

// OrderBy appends terms to q's ORDER BY list. It copies q.order into a
// FRESH backing array before appending -- never a bare
// append(q.order, ...) -- so two Query values branched from one base never
// share or corrupt each other's order slice.
func (q Query[T, PT]) OrderBy(terms ...OrderTerm[T]) Query[T, PT] {
	next := make([]OrderTerm[T], 0, len(q.order)+len(terms))
	next = append(next, q.order...)
	next = append(next, terms...)
	q.order = next

	return q
}

// Limit sets q's row limit. A value of 0 (the zero value, also what an
// unset Limit produces) renders NO LIMIT clause -- i.e. unlimited rows --
// matching the zero-value-is-unset convention. To cap rows use n > 0; a
// caller wanting "exactly zero rows" should not use Limit(0) (it means
// unlimited, not an empty result).
func (q Query[T, PT]) Limit(n int) Query[T, PT] {
	q.limit = n

	return q
}

// Offset sets q's row offset. A value of 0 (the zero value) renders no
// OFFSET clause, matching Limit's zero-value-is-unset convention.
func (q Query[T, PT]) Offset(n int) Query[T, PT] {
	q.offset = n

	return q
}

// Distinct makes q render `SELECT DISTINCT`, removing duplicate result rows.
// It is a copy-on-write method like Where/OrderBy: the receiver is left
// unmodified. DISTINCT is supported by every dialect and is deliberately
// not capability-gated. It may not be combined with ForUpdate/ForShare --
// see ErrLockingWithDistinct.
func (q Query[T, PT]) Distinct() Query[T, PT] {
	q.distinct = true

	return q
}

// DistinctOn makes q render Postgres's `SELECT DISTINCT ON (cols)`, keeping
// the first row of each distinct `cols` combination. Combine with OrderBy to
// make "first" deterministic. The columns must be codegen-derived
// AnyColumn[T] values erased via Col(), never raw strings. It is a
// copy-on-write method, and supersedes Distinct in the rendered prefix.
//
// DISTINCT ON is a Postgres extension: dialects without it return a typed
// dialect.ErrUnsupportedByDialect. Like DISTINCT, it may not be combined with
// a row lock (ErrLockingWithDistinct).
func (q Query[T, PT]) DistinctOn(cols ...AnyColumn[T]) Query[T, PT] {
	q.distinctOn = anyColumnNames(cols)

	return q
}

// Tablesample makes q render a Postgres `TABLESAMPLE <method> (<arg>)` FROM
// clause, returning an approximate random sample of the table's blocks or
// rows. method is one of "SYSTEM" or "BERNOULLI" (case-insensitive); any
// other method is a rendering-time error, never interpolated into SQL. arg is
// the sampling percentage in [0, 100]. It is Postgres-only (other dialects
// return a typed dialect.ErrUnsupportedByDialect) and copy-on-write.
func (q Query[T, PT]) Tablesample(method string, arg float64) Query[T, PT] {
	q.tablesample = &Tablesample{Method: method, Arg: arg}

	return q
}

// ForUpdate makes q render a `FOR UPDATE` row-lock suffix, acquiring an
// exclusive lock on the selected rows (the worker/queue "claim" pattern).
// The lock is emitted after ORDER BY/LIMIT/OFFSET; combine with NoWait or
// SkipLocked for the non-blocking variants. Requires dialect.LockingDialect
// support -- SQLite, for example, rejects it with a typed
// dialect.ErrUnsupportedByDialect. It is a copy-on-write method.
func (q Query[T, PT]) ForUpdate() Query[T, PT] {
	q.lock = LockForUpdate

	return q
}

// ForShare makes q render a `FOR SHARE` row-lock suffix, acquiring a shared
// lock on the selected rows. See ForUpdate for the capability gate and the
// copy-on-write rule.
func (q Query[T, PT]) ForShare() Query[T, PT] {
	q.lock = LockForShare

	return q
}

// ForNoKeyUpdate makes q render a Postgres `FOR NO KEY UPDATE` row-lock
// suffix: a weaker exclusive lock than FOR UPDATE that does not block
// foreign-key checks. It is Postgres-only; other dialects reject it with a
// typed dialect.ErrUnsupportedByDialect (see ExtendedLockingDialect). It is a
// copy-on-write method.
func (q Query[T, PT]) ForNoKeyUpdate() Query[T, PT] {
	q.lock = LockForNoKeyUpdate

	return q
}

// ForKeyShare makes q render a Postgres `FOR KEY SHARE` row-lock suffix: the
// weakest row lock, which blocks key-changing updates but not others. It is
// Postgres-only; other dialects reject it with a typed
// dialect.ErrUnsupportedByDialect. It is a copy-on-write method.
func (q Query[T, PT]) ForKeyShare() Query[T, PT] {
	q.lock = LockForKeyShare

	return q
}

// ForUpdateOf makes q render a Postgres `FOR UPDATE OF <tables>` row-lock
// suffix, restricting the lock to the given columns' tables so rows of other
// joined tables are not locked. Postgres's OF list names TABLES, not
// columns, so the distinct home table of each column is used (e.g.
// ForUpdateOf(Users.ID) -> `FOR UPDATE OF "users"`). It sets the FOR UPDATE
// lock mode, is Postgres-only (other dialects return a typed
// dialect.ErrUnsupportedByDialect), and is copy-on-write.
func (q Query[T, PT]) ForUpdateOf(cols ...AnyColumn[T]) Query[T, PT] {
	q.lock = LockForUpdate
	q.lockOf = anyColumnTables(cols)

	return q
}

// anyColumnTables returns the distinct, order-preserving home table names of
// cols, dropping any column whose table is unknown (an empty name would
// render an invalid identifier).
func anyColumnTables[T any](cols []AnyColumn[T]) []string {
	out := make([]string, 0, len(cols))
	seen := make(map[string]struct{}, len(cols))

	for _, c := range cols {
		t := c.Table()
		if t == "" {
			continue
		}

		if _, ok := seen[t]; ok {
			continue
		}

		seen[t] = struct{}{}

		out = append(out, t)
	}

	return out
}

// NoWait appends `NOWAIT` to the row-lock clause, making the statement fail
// immediately if a requested row is locked instead of waiting. It only has
// an effect with ForUpdate/ForShare set; used alone it is
// ErrLockingRequiresLockMode. Calling NoWait then SkipLocked (or vice versa)
// is last-one-wins, since the two are mutually exclusive.
func (q Query[T, PT]) NoWait() Query[T, PT] {
	q.nowait = true
	q.skipLocked = false

	return q
}

// SkipLocked appends `SKIP LOCKED` to the row-lock clause, excluding rows
// locked by other transactions from the result instead of waiting. It only
// has an effect with ForUpdate/ForShare set; used alone it is
// ErrLockingRequiresLockMode. Calling SkipLocked then NoWait (or vice versa) is
// last-one-wins, since the two are mutually exclusive.
func (q Query[T, PT]) SkipLocked() Query[T, PT] {
	q.skipLocked = true
	q.nowait = false

	return q
}

// selectModifiers erases q's SELECT modifiers into the renderer's shape.
func (q Query[T, PT]) selectModifiers() render.SelectModifiers {
	mods := render.SelectModifiers{
		Distinct:   q.distinct,
		DistinctOn: q.distinctOn,
		Lock:       render.LockMode(q.lock),
		LockOf:     q.lockOf,
		NoWait:     q.nowait,
		SkipLocked: q.skipLocked,
	}

	if q.tablesample != nil {
		mods.Tablesample = render.Tablesample{Method: q.tablesample.Method, Arg: q.tablesample.Arg}
	}

	return mods
}

// validateSelectLocking rejects invalid modifier combinations and lock modes
// the resolved dialect lacks, before any SQL is rendered. The dialect-
// independent checks (DISTINCT + lock, and a lock modifier with no lock
// mode) run first; the capability checks then report a typed
// dialect.ErrUnsupportedByDialect for a dialect that can't run the clause.
func (q Query[T, PT]) validateSelectLocking(d dialect.Dialect) error {
	return validateSelectModifiers(d, q.distinct, q.distinctOn, q.lock, q.lockOf, q.nowait, q.skipLocked)
}

// validateSelectModifiers is the shared, receiver-free body of the selector
// modifier gate, used by both the plain and projecting SELECT paths, whose
// SELECT modifiers follow identical rules.
func validateSelectModifiers(d dialect.Dialect, distinct bool, distinctOn []string, lock LockMode, lockOf []string, nowait, skipLocked bool) error {
	if (distinct || distinctOn != nil) && lock != LockNone {
		return fmt.Errorf("orm: Query: %w", ErrLockingWithDistinct)
	}

	if err := validateDistinctOn(d, distinctOn); err != nil {
		return err
	}

	if lock == LockNone {
		if nowait || skipLocked || len(lockOf) > 0 {
			return fmt.Errorf("orm: Query: %w", ErrLockingRequiresLockMode)
		}

		return nil
	}

	return validateLockMode(d, lock, lockOf, nowait, skipLocked)
}

// validateDistinctOn gates a DISTINCT ON request on the dialect capability;
// a nil list is unset (no check), an explicitly empty list is a caller error.
func validateDistinctOn(d dialect.Dialect, distinctOn []string) error {
	if distinctOn == nil {
		return nil
	}

	if len(distinctOn) == 0 {
		return errors.New("orm: Query: DISTINCT ON requires at least one column")
	}

	dd, ok := d.(dialect.DistinctOnDialect)
	if !ok || !dd.SupportsDistinctOn() {
		return fmt.Errorf("orm: %w: dialect %q does not support DISTINCT ON", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// validateLockMode gates a non-zero lock mode and its NOWAIT/SKIP LOCKED/
// FOR UPDATE OF modifiers on the dialect's locking capabilities.
func validateLockMode(d dialect.Dialect, lock LockMode, lockOf []string, nowait, skipLocked bool) error {
	ld, ok := d.(dialect.LockingDialect)
	if !ok {
		return fmt.Errorf("orm: %w: dialect %q does not support row locking", dialect.ErrUnsupportedByDialect, d.Name())
	}

	switch lock { //nolint:exhaustive // LockNone handled by the caller; out-of-range cannot reach here
	case LockForUpdate:
		if !ld.SupportsForUpdate() {
			return fmt.Errorf("orm: %w: dialect %q does not support FOR UPDATE", dialect.ErrUnsupportedByDialect, d.Name())
		}
	case LockForShare:
		if !ld.SupportsForShare() {
			return fmt.Errorf("orm: %w: dialect %q does not support FOR SHARE", dialect.ErrUnsupportedByDialect, d.Name())
		}
	case LockForNoKeyUpdate, LockForKeyShare:
		if !extendedLockSupported(d, lock) {
			return fmt.Errorf("orm: %w: dialect %q does not support lock mode %d", dialect.ErrUnsupportedByDialect, d.Name(), lock)
		}
	}

	if len(lockOf) > 0 && !supportsForUpdateOf(d) {
		return fmt.Errorf("orm: %w: dialect %q does not support FOR UPDATE OF", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if nowait && !ld.SupportsNoWait() {
		return fmt.Errorf("orm: %w: dialect %q does not support NOWAIT", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if skipLocked && !ld.SupportsSkipLocked() {
		return fmt.Errorf("orm: %w: dialect %q does not support SKIP LOCKED", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// extendedLockSupported reports whether d implements ExtendedLockingDialect
// and supports the given weaker Postgres lock strength.
func extendedLockSupported(d dialect.Dialect, lock LockMode) bool {
	ext, ok := d.(dialect.ExtendedLockingDialect)
	if !ok {
		return false
	}

	switch lock { //nolint:exhaustive // only the two extended modes reach here
	case LockForNoKeyUpdate:
		return ext.SupportsForNoKeyUpdate()
	case LockForKeyShare:
		return ext.SupportsForKeyShare()
	default:
		return false
	}
}

// supportsForUpdateOf reports whether d implements ExtendedLockingDialect and
// supports the FOR ... OF target list.
func supportsForUpdateOf(d dialect.Dialect) bool {
	ext, ok := d.(dialect.ExtendedLockingDialect)

	return ok && ext.SupportsForUpdateOf()
}

// resolveDialect turns exec's own Dialect() name into a dialect.Dialect.
// It never silently falls back for an unrecognized name; an unknown name
// returns a clear error instead of a silently-wrong default.
func resolveDialect(exec db.DB) (dialect.Dialect, error) {
	d, err := dialect.For(exec.Dialect())
	if err != nil {
		return nil, fmt.Errorf("orm: %w", err)
	}

	return d, nil
}

// encodeArgs converts a bound time.Time value to RFC3339 UTC text when the
// dialect is SQLite, before it reaches the driver. The modernc sqlite driver
// otherwise persists a time.Time using Go's time.Time.String() format
// ("2006-01-02 15:04:05 +0000 UTC"), which generated Scan methods --
// which parse timestamp columns as RFC3339 text -- cannot scan back.
// Encoding here, at orm's own execution boundary and only for the sqlite
// dialect, keeps every orm statement consistent with its documented RFC3339
// text storage contract without touching the driver (whose default
// time.Time format other, non-orm consumers still rely on) or the Postgres
// path (which binds time.Time natively).
func encodeArgs(d dialect.Dialect, args []any) []any {
	if d.Name() != "sqlite" {
		return args
	}

	changed := false

	for _, a := range args {
		if _, ok := a.(time.Time); ok {
			changed = true

			break
		}
	}

	if !changed {
		return args
	}

	out := make([]any, len(args))

	for i, a := range args {
		if t, ok := a.(time.Time); ok {
			out[i] = t.UTC().Format(time.RFC3339)
		} else {
			out[i] = a
		}
	}

	return out
}

func toRenderNode[T any](n Node) render.Node {
	children := make([]render.Node, len(n.Children))
	for i, c := range n.Children {
		children[i] = toRenderNode[T](c)
	}

	var j *render.JSONExpr

	if n.JSON != nil {
		steps := make([]render.JSONStep, len(n.JSON.Steps))
		for i, s := range n.JSON.Steps {
			steps[i] = render.JSONStep{Key: s.Key, Index: s.Index, IsIndex: s.IsIndex}
		}

		j = &render.JSONExpr{Op: render.JSONOp(n.JSON.Op), Steps: steps, Path: n.JSON.Path}
	}

	var f *render.FTSExpr

	if n.FTS != nil {
		cols := n.FTS.Columns
		if len(cols) > 0 {
			cols = append([]string(nil), cols...)
		}

		f = &render.FTSExpr{Op: render.FTSOp(n.FTS.Op), Query: n.FTS.Query, Mode: render.FTSMode(n.FTS.Mode), Columns: cols}
	}

	value := n.Value

	if re, ok := n.Value.(RawExpr); ok {
		// Value is an `any`, so an orm.RawExpr cannot flow through untouched
		// to render's KindRaw handler (which type-asserts its own RawExpr).
		// Convert with a plain field-for-field literal, mirroring the JSON/
		// FTS conversion above.
		value = render.RawExpr{Fragment: re.Fragment, Args: re.Args}
	} else if sq, ok := n.Value.(subquery); ok {
		// A subquery node's Value is an orm.subquery -- converted, like the
		// RawExpr above, to render's own erased shape just before the render
		// pass. The inner table/columns/where/order were already snapshotted
		// and shape-converted at predicate-construction time; only the SQL
		// TEXT remains (produced at execution time against the enclosing
		// query's dialect).
		value = render.Subquery{
			Table:   sq.table,
			Columns: sq.columns,
			Where:   sq.where,
			Order:   sq.order,
			Limit:   sq.limit,
			Offset:  sq.offset,
		}
	} else if ref, ok := n.Value.(outerRefCore); ok {
		// A correlated reference's Value is an OuterRef[U,V] -- a
		// generic type whose exact instantiation is erased here, through
		// the small outerRefCore interface every instantiation satisfies.
		// Only the outer column's home TABLE/NAME travel to render: the V
		// cell type was already compile-time-validated by the comparison
		// method that built the marker, and the actual enclosing table name
		// is a property of the renderer's runtime scope, not of the marker.
		value = render.OuterRef{Table: ref.outerRefTable(), Column: ref.outerRefName()}
	}

	return render.Node{
		Kind:     render.NodeKind(n.Kind),
		Table:    n.Table,
		Column:   n.Column,
		Op:       render.Op(n.Op),
		Value:    value,
		Compound: render.CompoundOp(n.Compound),
		Children: children,
		Tuple:    n.Tuple,
		JSON:     j,
		FTS:      f,
		Func:     toRenderFunc[T](n.Func),
	}
}

// toRenderFunc erases a FuncExpr tree into render's own shape,
// recursively converting every nested argument/when/else Node. It mirrors
// the JSON/FTS conversions above (a plain field-for-field literal), keeping
// render decoupled from orm. A nil input yields nil.
func toRenderFunc[T any](fn *FuncExpr) *render.FuncExpr {
	if fn == nil {
		return nil
	}

	out := &render.FuncExpr{Name: fn.Name, Case: fn.Case}

	if len(fn.Args) > 0 {
		out.Args = make([]render.Node, len(fn.Args))
		for i, a := range fn.Args {
			out.Args[i] = toRenderNode[T](a)
		}
	}

	if len(fn.Whens) > 0 {
		out.Whens = make([]render.FuncWhen, len(fn.Whens))
		for i, w := range fn.Whens {
			out.Whens[i] = render.FuncWhen{Cond: toRenderNode[T](w.Cond), Then: toRenderNode[T](w.Then)}
		}
	}

	if fn.Else != nil {
		e := toRenderNode[T](*fn.Else)
		out.Else = &e
	}

	return out
}

func toRenderOrder[T any](order []OrderTerm[T]) []render.OrderTerm {
	out := make([]render.OrderTerm, len(order))
	for i, o := range order {
		var f *render.FTSExpr

		if o.FTS != nil {
			cols := o.FTS.Columns
			if len(cols) > 0 {
				cols = append([]string(nil), cols...)
			}

			f = &render.FTSExpr{Op: render.FTSOp(o.FTS.Op), Query: o.FTS.Query, Mode: render.FTSMode(o.FTS.Mode), Columns: cols}
		}

		out[i] = render.OrderTerm{Column: o.Column.Name(), Table: o.Column.Table(), Desc: o.Desc, Nulls: render.NullsOrder(o.Nulls), FTS: f, Func: toRenderFunc[T](o.Func)}
	}

	return out
}

// All runs q against exec and returns every matching row, scanned via T's
// codegen'd Scan method with zero reflection. It is a thin materializing
// wrapper over Stream: All and Stream share one execution path, and All
// just drains the iterator into a slice. See ExampleQuery_All for a
// runnable example.
func (q Query[T, PT]) All(ctx context.Context, exec db.DB) ([]PT, error) {
	var out []PT

	// A positive Limit bounds the row count in advance, so pre-size out to
	// avoid append's repeated-doubling growth from nil. q.limit == 0 means
	// unlimited (see Limit's doc comment), so it's left nil-start as
	// before -- guessing a capacity for an unbounded query could
	// over-allocate for a query that returns few rows.
	if q.limit > 0 {
		out = make([]PT, 0, q.limit)
	}

	for row, err := range q.Stream(ctx, exec) {
		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}

	return out, nil
}

// First runs q with an implicit LIMIT 1 and returns the first matching row,
// or ok=false if there was none.
func (q Query[T, PT]) First(ctx context.Context, exec db.DB) (row PT, ok bool, err error) {
	rows, err := q.Limit(1).All(ctx, exec)
	if err != nil {
		return nil, false, err
	}

	if len(rows) == 0 {
		return nil, false, nil
	}

	return rows[0], true, nil
}

// FirstOrErr runs q with an implicit LIMIT 1 and returns the first matching
// row, or wraps db.ErrNotFound (testable with errors.Is) when no row
// matches. It is the ergonomic error-returning sibling of First -- the
// common "fetch by id or fail" call shape -- and shares First's hot path.
func (q Query[T, PT]) FirstOrErr(ctx context.Context, exec db.DB) (PT, error) {
	row, ok, err := q.First(ctx, exec)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, fmt.Errorf("orm: Query.FirstOrErr: %w", db.ErrNotFound)
	}

	return row, nil
}

// Count runs `SELECT COUNT(*)` for q's table/filter, ignoring order/limit/
// offset (meaningless for a count).
func (q Query[T, PT]) Count(ctx context.Context, exec db.DB) (int64, error) {
	if q.lock != LockNone || q.nowait || q.skipLocked {
		return 0, fmt.Errorf("orm: Query.Count: %w", ErrLockingNotSelect)
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	query, args, err := render.Count(d, q.table.Name(), toRenderNode[T](q.where.Render()))
	if err != nil {
		return 0, fmt.Errorf("orm: Query.Count: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	rows, err := queryRows(ctx, exec, query, encoded)
	if err != nil {
		return 0, fmt.Errorf("orm: Query.Count: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var n int64

	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, fmt.Errorf("orm: Query.Count: scan: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("orm: Query.Count: %w", err)
	}

	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("orm: Query.Count: %w", err)
	}

	return n, nil
}

// Exists reports whether q's filter matches at least one row.
func (q Query[T, PT]) Exists(ctx context.Context, exec db.DB) (bool, error) {
	n, err := q.Limit(1).Count(ctx, exec)
	if err != nil {
		return false, err
	}

	return n > 0, nil
}
