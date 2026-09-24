package orm

import "github.com/zenta-dev/zever/orm/render"

// Op identifies a comparison operator applied to a single column. The zero
// value Eq is the equality comparison.
//
// Op is a type alias for render.Op (not just a mirrored copy): orm imports
// render (never the other way -- see render's package doc), so aliasing is
// safe and, unlike a separately-defined mirror kept in sync only by
// convention, makes a reordering/insertion mistake in either package's
// constant list a compile error at every call site that converts between
// them, instead of a silent SQL-rendering bug.
type Op = render.Op

// Supported comparison operators. These re-export render's OpXxx constants
// under orm's own builder-facing names; see render.Op's constants for the
// full per-value documentation (Between's [2]any convention, EqAny/NeqAny/
// EqAll/NeqAll's Postgres-only ANY/ALL quantifiers, etc.).
const (
	Eq        = render.OpEq
	Neq       = render.OpNeq
	Gt        = render.OpGt
	Gte       = render.OpGte
	Lt        = render.OpLt
	Lte       = render.OpLte
	In        = render.OpIn
	Like      = render.OpLike
	IsNull    = render.OpIsNull
	IsNotNull = render.OpIsNotNull
	Between   = render.OpBetween
	NotIn     = render.OpNotIn
	EqAny     = render.OpEqAny
	NeqAny    = render.OpNeqAny
	EqAll     = render.OpEqAll
	NeqAll    = render.OpNeqAll
)

// CompoundOp identifies how child nodes of an NCompound Node are combined.
// Alias for render.CompoundOp; see Op's doc comment for why aliasing (not
// mirroring) is safe here.
type CompoundOp = render.CompoundOp

// Supported compound operators, re-exporting render's CompoundXxx constants
// under orm's own names.
const (
	CAnd = render.CompoundAnd
	COr  = render.CompoundOr
	CNot = render.CompoundNot
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
//
// NodeKind is a type alias for render.NodeKind; see Op's doc comment for
// why aliasing (not mirroring) is safe here.
type NodeKind = render.NodeKind

// Supported node kinds, re-exporting render's KindXxx constants under orm's
// own names; see render.NodeKind's constants for the full per-value
// documentation (NJSON/NFTS/NTuple/NArray's payload conventions).
const (
	NNone     = render.KindNone
	NLit      = render.KindLit
	NColumn   = render.KindColumn
	NUnary    = render.KindUnary
	NBinary   = render.KindBinary
	NIn       = render.KindIn
	NBetween  = render.KindBetween
	NLike     = render.KindLike
	NCompound = render.KindCompound
	NFunc     = render.KindFunc
	NSubquery = render.KindSubquery
	NRaw      = render.KindRaw
	NJSON     = render.KindJSON
	NFTS      = render.KindFTS
	NTuple    = render.KindTuple
	NArray    = render.KindArray
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
