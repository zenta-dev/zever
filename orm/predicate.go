package orm

// Op identifies a comparison operator applied to a single column. The zero
// value Eq is the equality comparison.
type Op int

// Supported comparison operators.
const (
	Eq Op = iota
	Neq
	Gt
	Gte
	Lt
	Lte
	In
	Like
	IsNull
	IsNotNull
	// Between is a `column BETWEEN lo AND hi` predicate. Its Node.Value is
	// always a [2]any{lo, hi} fixed array, never a []any slice -- the same
	// convention the value-list In avoids, letting the renderer
	// distinguish a Between node from an In node by a plain type assertion.
	Between
	// NotIn is a `column NOT IN (subquery)` predicate -- an NIn node whose
	// Op is NotIn, built by Column.NotInSub/NullableColumn.NotInSub. The
	// renderer emits the NOT IN keyword instead of IN for this Op; a
	// value-list negation is still spelled Not(col.In(vs...)).
	NotIn
	// EqAny is a `column = ANY(array)` predicate (Postgres): true when the
	// column equals at least one element of the bound value list. Its node
	// is NArray -- never NBinary -- because the ANY/ALL quantifier wraps the
	// whole right-hand list, which is not expressible as a scalar compare.
	// Built by Column.EqAny; a non-Postgres dialect returns a typed
	// dialect.ErrUnsupportedByDialect.
	EqAny
	// NeqAny is a `column <> ANY(array)` predicate (Postgres): true when the
	// column differs from at least one element of the bound value list.
	// Built by Column.NeqAny.
	NeqAny
	// EqAll is a `column = ALL(array)` predicate (Postgres): true when the
	// column equals every element of the bound value list. Built by
	// Column.EqAll.
	EqAll
	// NeqAll is a `column <> ALL(array)` predicate (Postgres): true when the
	// column differs from every element of the bound value list -- the
	// idiomatic NOT IN equivalent that is NULL-safe for the empty list.
	// Built by Column.NeqAll.
	NeqAll
)

// CompoundOp identifies how child nodes of an NCompound Node are combined.
type CompoundOp int

// Supported compound operators.
const (
	CAnd CompoundOp = iota
	COr
	CNot
)

// NodeKind discriminates the shape of a Node. The zero value, NNone, means
// "no predicate" -- Predicate[T]'s zero value renders to a Node with this
// Kind, and render treats it as "omit this clause".
//
// NLit/NColumn/NFunc are the scalar-expression algebra built by orm/expr.go:
// an NFunc node carries a FuncExpr tree and is rendered either as a
// comparison predicate (its Op/Value) or -- nested in another expression's
// argument list -- as a bare scalar. NUnary is reserved for future use;
// unused today.
type NodeKind int

// Supported node kinds.
const (
	NNone NodeKind = iota
	NLit
	NColumn
	// NUnary is reserved for a future phase (e.g. IS DISTINCT FROM-style
	// unary wrapping); unused today.
	NUnary
	NBinary
	NIn
	NBetween
	NLike
	NCompound
	// NFunc is a scalar function/conditional expression node (orm/expr.go):
	// its Func field carries the erased function tree. As a standalone
	// predicate it applies Op/Value as the comparison against the
	// expression's result (`COALESCE(bio, ?) = ?`); nested inside another
	// expression's Args it is rendered as a bare scalar.
	NFunc
	// NSubquery is a subquery predicate node whose Value is a subquery --
	// the erased inner SELECT. It renders as `EXISTS (SELECT ...)` for
	// Exists/NotExists, and the same subquery value travels inside an
	// NIn node's Value (In/NotIn-subquery: `col [NOT] IN (SELECT ...)`) and
	// an NBinary node's Value (scalar comparisons: `col OP (SELECT ...)`).
	NSubquery
	NRaw
	// NJSON is a JSON operator predicate: its LHS is a JSON
	// expression over the base column (see Node.JSON), rendered per dialect
	// as jsonb operators (postgres) or json1 functions (sqlite).
	NJSON
	// NFTS is a full-text-search expression over the base column,
	// rendered per dialect as tsvector machinery on postgres
	// and FTS5 MATCH on sqlite. See Node.FTS and render/fts.go.
	NFTS
	// NTuple is a multi-column row-value predicate: a tuple of outer columns
	// compared against a subquery -- `(a, b) [NOT] IN (SELECT ...)` (Op
	// In/NotIn) or `(a, b) OP (SELECT ...)` for Op Eq..Lte (row-value
	// comparison). Node.Tuple carries the LHS column list and Node.Value the
	// inner subquery. Built by orm/tuple.go's Tuple methods.
	NTuple
	// NArray is a Postgres array-quantifier predicate over a single column:
	// `column = ANY(...)` / `<> ANY(...)` / `= ALL(...)` / `<> ALL(...)`.
	// Node.Value is always a []any of bound elements (never a scalar), and
	// Node.Op selects both the comparison (=/ <>) and the quantifier
	// (ANY/ALL). Built by Column.EqAny/NeqAny/EqAll/NeqAll; a non-Postgres
	// dialect returns a typed dialect.ErrUnsupportedByDialect at render
	// time. See render/array.go.
	NArray
)

// Node is the erased, dialect-agnostic shape of one predicate tree, walked
// directly by render. It carries no rendering logic itself.
type Node struct {
	Kind   NodeKind
	Table  string
	Column string
	Op     Op
	Value  any

	Compound CompoundOp
	Children []Node

	// Tuple is the LHS column list of an NTuple row-value predicate -- the
	// `(a, b)` of `(a, b) [NOT] IN (SELECT ...)` / `(a, b) OP (SELECT ...)`
	// -- and is empty for every other kind. Columns are codegen-derived
	// names erased through AnyColumn.Col(), never caller strings. The inner
	// subquery travels in Value; see orm/tuple.go.
	Tuple []string

	// JSON is non-nil only when Kind == NJSON: the JSON operator applied to
	// the base column (Table/Column above), built by the JSON dialect
	// helpers and rendered per dialect by render.
	JSON *JSONExpr

	// FTS is non-nil only when Kind == NFTS: the full-text-search operator
	// and search text applied to the base column (Table/Column above), built
	// by the FTS dialect helpers and rendered per dialect by render.
	FTS *FTSExpr

	// Func is non-nil only when Kind == NFunc: the scalar function or CASE
	// expression tree built by orm/expr.go. The node's Op/Value apply only
	// at the top level (the comparison predicate); nested occurrences are
	// reached through Func.Args.
	Func *FuncExpr
}

// Predicate is a typed predicate expression over entity T. Its zero
// value is "unset" (IsSet reports false, Render's Node.Kind is NNone), so a
// Query[T]'s where field defaults to "no filter" without needing a pointer
// or an interface nil check.
type Predicate[T any] struct{ n Node }

// Render returns p's erased Node -- the shape render consumes. It is
// exported so the render package can consume it across the package
// boundary, but it is meant for internal/render use only: ordinary calling
// code should never need to inspect a Predicate's Node directly.
func (p Predicate[T]) Render() Node { return p.n }

// IsSet reports whether p carries an actual predicate.
func (p Predicate[T]) IsSet() bool { return p.n.Kind != NNone }

// And combines ps with logical AND. Unset predicates are skipped; And()
// with zero set predicates returns the zero Predicate[T] (unset), and a
// single set predicate is returned unwrapped rather than as a degenerate
// one-child AND.
func And[T any](ps ...Predicate[T]) Predicate[T] { return combine[T](CAnd, ps) }

// Or combines ps with logical OR; see And for the zero/one-element rules.
func Or[T any](ps ...Predicate[T]) Predicate[T] { return combine[T](COr, ps) }

// Not negates p. Not of an unset predicate is itself unset.
func Not[T any](p Predicate[T]) Predicate[T] {
	if !p.IsSet() {
		return Predicate[T]{}
	}

	return Predicate[T]{n: Node{Kind: NCompound, Compound: CNot, Children: []Node{p.n}}}
}

func combine[T any](op CompoundOp, ps []Predicate[T]) Predicate[T] {
	children := make([]Node, 0, len(ps))

	for _, p := range ps {
		if p.IsSet() {
			children = append(children, p.n)
		}
	}

	switch len(children) {
	case 0:
		return Predicate[T]{}
	case 1:
		return Predicate[T]{n: children[0]}
	default:
		return Predicate[T]{n: Node{Kind: NCompound, Compound: op, Children: children}}
	}
}

// Exists builds an `EXISTS (SELECT ...)` predicate over an inner query.
// T is the OUTER entity the predicate will be combined into (e.g. via
// Query[T, PT].Where or And/Or); it appears in no argument, so it must be
// given explicitly at the call site -- the inner entity types C/PC are
// inferred from inner:
//
//	orm.Exists[widget](From(widgetOrders).Where(...))
//
// The inner query renders through the SAME dialect as the enclosing one at
// execution time, with its bound arguments numbered after any outer
// predicate arguments that precede it in the SQL text. Unless the inner
// query's own Where references a column of the enclosing query through
// Outer/OuterNullable (a correlated EXISTS, the classic
// `NOT EXISTS (... inner.col = outer.col ...)` anti-join), the inner query
// is evaluated independently and EXISTS has the same truth value for every
// outer row. NotExists is spelled through Not below.
func Exists[T any, C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NSubquery, Value: toSubquery(inner)}}
}

// NotExists builds a `NOT EXISTS (SELECT ...)` predicate over an inner
// query; see Exists for the T/C/PC type-argument story. It is Exists
// wrapped in the same Negation Not() applies to any predicate, so it
// composes everywhere a negated predicate does.
func NotExists[T any, C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return Not(Exists[T](inner))
}
