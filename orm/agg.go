package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/render"
)

// AggFunc identifies a SQL aggregate function. Type alias for
// render.AggFunc (orm imports render, never the reverse -- see orm.Op's
// doc comment for why aliasing beats a separately-kept-in-sync mirror).
type AggFunc = render.AggFunc

// Supported aggregate functions. AggArrayAgg/AggStringAgg/AggGroupConcat are
// the ordered-argument (concatenating/collecting) aggregates; each is gated
// per dialect (see dialect.OrderedAggregateDialect). These re-export
// render's identically-named constants.
const (
	AggCount       = render.AggCount
	AggSum         = render.AggSum
	AggAvg         = render.AggAvg
	AggMin         = render.AggMin
	AggMax         = render.AggMax
	AggArrayAgg    = render.AggArrayAgg
	AggStringAgg   = render.AggStringAgg
	AggGroupConcat = render.AggGroupConcat
)

// Aggregate describes one aggregate expression in a GroupedQuery's select
// list, e.g. Sum(orderAmount) renders as `SUM("amount") AS "sum_amount"`.
//
// Aggregate is deliberately a plain, non-generic struct, matching the
// legacy ORM engine's Aggregate pattern: a single GroupBy(...).Agg(a, b, c)
// call must be able to hold aggregates built from columns of different value
// types (e.g. Sum over an int64 column and Min over a string column) in one
// homogeneous []Aggregate slice, which a generic Aggregate[V] could not do
// without either boxing (defeating the point) or a sum type. The cost of
// this erasure is paid explicitly by HavingPredicate -- see its doc comment.
//
// The aggregate extensions the FILTER/DISTINCT builders attach travel in
// unexported fields (distinct/filter) so the struct stays comparable -- the
// constructor tests and any caller inspecting an Aggregate by value keep
// working.
type Aggregate struct {
	Func   AggFunc
	Column string
	Alias  string

	distinct bool
	filter   *Node

	// delim/hasDelim carry the optional string_agg/group_concat delimiter.
	// hasDelim distinguishes "no delimiter" (SQLite/MySQL's own default) from
	// a deliberately empty delimiter. bool/string keep Aggregate comparable.
	delim    string
	hasDelim bool

	// ordered carries the argument-last ORDER BY of an ordered aggregate. It
	// is a pointer so Aggregate stays comparable (the existing constructor
	// tests compare Aggregate values with ==).
	ordered *orderedArgs

	// bad is set when a constructor/OrderBy received input that cannot render
	// (an out-of-range delimiter count or a non-identifier order term). The
	// renderer turns it into a typed error -- never a panic and never
	// silently-wrong SQL.
	bad string
}

// orderRef is the erased shape of one in-aggregate ORDER BY term: an
// identifier plus its direction. Ordered aggregates accept only plain columns,
// never expressions or FTS terms.
type orderRef struct {
	column string
	desc   bool
}

// orderedArgs is the erased argument-last ORDER BY list of an ordered
// aggregate.
type orderedArgs struct{ terms []orderRef }

// Count builds a COUNT(*) aggregate.
func Count() Aggregate { return Aggregate{Func: AggCount, Alias: "count"} }

// CountDistinct builds a COUNT(DISTINCT c) aggregate over the erased column
// reference c. It is universal SQL (every supported dialect accepts
// COUNT(DISTINCT ...)), so it is deliberately not capability-gated.
func CountDistinct[T any](c AnyColumn[T]) Aggregate {
	return Aggregate{Func: AggCount, Column: c.Name(), Alias: "count_distinct_" + c.Name(), distinct: true}
}

// Distinct returns a copy of a that renders its argument with the DISTINCT
// modifier (`COUNT(DISTINCT c)`, `SUM(DISTINCT c)`). It leaves the receiver
// unmodified, matching every other builder's copy-on-write discipline. A
// DISTINCT aggregate with no column is a rendering-time error, never
// `COUNT(DISTINCT *)`.
func (a Aggregate) Distinct() Aggregate {
	a.distinct = true

	return a
}

// predicateSource is the minimal shape an aggregate FILTER predicate must
// have: Predicate[T]'s exported Render method (its signature carries no type
// parameters), so a Predicate of any entity T satisfies it. Aggregate has
// already erased its column's value type by construction time, so the
// FILTER predicate's T cannot be pinned to it; the renderer type-checks the
// bound value at rendering time instead (never a panic).
type predicateSource interface{ Render() Node }

// Filter attaches a `FILTER (WHERE p)` clause to a, e.g.
// `COUNT(*) FILTER (WHERE "quantity" > $1)`. It leaves the receiver
// unmodified. Postgres and SQLite support the clause; MySQL has no FILTER
// syntax, so execution there returns the typed
// dialect.ErrUnsupportedByDialect -- never a silently-dropped filter.
func (a Aggregate) Filter(p predicateSource) Aggregate {
	n := p.Render()
	a.filter = &n

	return a
}

// Numeric constrains Sum/Avg's column value type to something a SQL SUM/AVG
// is actually meaningful over.
type Numeric interface {
	~int32 | ~int64 | ~float32 | ~float64
}

// Sum builds a SUM(c) aggregate. V is constrained to Numeric so a caller
// cannot accidentally SUM a string or timestamp column.
func Sum[V Numeric, T any](c Column[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggSum, Column: name, Alias: "sum_" + name}
}

// Avg builds an AVG(c) aggregate.
func Avg[V Numeric, T any](c Column[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggAvg, Column: name, Alias: "avg_" + name}
}

// Min builds a MIN(c) aggregate. Unlike Sum/Avg, MIN is meaningful over any
// orderable column (a string or timestamp column included), so V is
// unconstrained.
func Min[V any, T any](c Column[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggMin, Column: name, Alias: "min_" + name}
}

// Max builds a MAX(c) aggregate; see Min for why V is unconstrained.
func Max[V any, T any](c Column[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggMax, Column: name, Alias: "max_" + name}
}

// SumNullable builds a SUM(c) aggregate over a nullable column; SQL
// aggregates already skip NULLs, so this is purely a Go-generic-acceptance
// change from Sum, mirroring the legacy field.SumNullable's rationale
// exactly.
func SumNullable[V Numeric, T any](c NullableColumn[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggSum, Column: name, Alias: "sum_" + name}
}

// AvgNullable builds an AVG(c) aggregate over a nullable column.
func AvgNullable[V Numeric, T any](c NullableColumn[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggAvg, Column: name, Alias: "avg_" + name}
}

// MinNullable builds a MIN(c) aggregate over a nullable column.
func MinNullable[V any, T any](c NullableColumn[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggMin, Column: name, Alias: "min_" + name}
}

// MaxNullable builds a MAX(c) aggregate over a nullable column.
func MaxNullable[V any, T any](c NullableColumn[T, V]) Aggregate {
	name := c.Col().Name()

	return Aggregate{Func: AggMax, Column: name, Alias: "max_" + name}
}

// OrderedAggregate is a typed aggregate carrying an argument-last ORDER BY
// (e.g. `array_agg(x ORDER BY y)`). It embeds Aggregate, so every existing
// aggregate method (Filter, Distinct, the HavingPredicate comparisons)
// promotes unchanged, and Agg accepts it alongside a plain Aggregate (via the
// sealed aggregatable interface). OrderBy returns a plain Aggregate carrying
// the recorded order terms, so the rest of the aggregate surface composes
// after it.
//
// The embedded Aggregate is populated by ArrayAgg/StringAgg/GroupConcat; the
// zero value is not meaningful on its own.
type OrderedAggregate[T any] struct{ Aggregate }

// ArrayAgg builds a Postgres `array_agg(c [ORDER BY ...])` aggregate. It is
// Postgres-only: MySQL and SQLite have no array_agg, so execution there
// returns the typed dialect.ErrUnsupportedByDialect. Order terms are added
// with OrderBy and must be plain columns.
func ArrayAgg[T any, V any](c Column[T, V]) OrderedAggregate[T] {
	name := c.Col().Name()

	return OrderedAggregate[T]{Aggregate{Func: AggArrayAgg, Column: name, Alias: "array_agg_" + name}}
}

// StringAgg builds a Postgres `string_agg(c, delim [ORDER BY ...])`
// aggregate; delim is bound as a placeholder argument. It is Postgres-only:
// MySQL and SQLite have no string_agg (use GroupConcat there), so execution
// returns the typed dialect.ErrUnsupportedByDialect. Order terms are added
// with OrderBy and must be plain columns.
func StringAgg[T any, V any](c Column[T, V], delim string) OrderedAggregate[T] {
	name := c.Col().Name()

	return OrderedAggregate[T]{Aggregate{
		Func:     AggStringAgg,
		Column:   name,
		Alias:    "string_agg_" + name,
		delim:    delim,
		hasDelim: true,
	}}
}

// GroupConcat builds a SQLite `group_concat(c [, delim])` / MySQL
// `GROUP_CONCAT(c [ORDER BY ...] [SEPARATOR s])` aggregate. The delimiter is
// optional (pass at most one): when omitted, the dialect's own default
// separator applies. On Postgres (which has no group_concat) execution
// returns the typed dialect.ErrUnsupportedByDialect. Order terms are added
// with OrderBy and must be plain columns.
//
// The delimiter is a bound placeholder on SQLite; MySQL's grammar only
// accepts a string literal after SEPARATOR, so it is emitted there as a hex
// string literal (never treated as a second function argument, which MySQL
// would misread as a per-row concatenation).
func GroupConcat[T any, V any](c Column[T, V], delim ...string) OrderedAggregate[T] {
	name := c.Col().Name()
	a := Aggregate{Func: AggGroupConcat, Column: name, Alias: "group_concat_" + name}

	switch len(delim) {
	case 0:
	case 1:
		a.delim = delim[0]
		a.hasDelim = true
	default:
		a.bad = "group_concat accepts at most one delimiter"
	}

	return OrderedAggregate[T]{a}
}

// OrderBy records an argument-last ORDER BY on an ordered aggregate and
// returns it as a plain Aggregate (so Filter/Distinct/HAVING comparisons
// compose after it). Every term must be a plain column reference (built from
// a schema-derived Column's Asc/Desc); an expression, FTS or NULLS-ordering
// term is a typed rendering error, never silently dropped. It leaves the
// receiver unmodified.
func (o OrderedAggregate[T]) OrderBy(terms ...OrderTerm[T]) Aggregate {
	a := o.Aggregate
	if a.bad != "" {
		a.ordered = &orderedArgs{}

		return a
	}

	refs := make([]orderRef, 0, len(terms))

	for _, t := range terms {
		if t.Func != nil || t.FTS != nil || t.Nulls != NullsDefault {
			a.bad = "ordered aggregate ORDER BY supports only plain column terms"
			a.ordered = &orderedArgs{}

			return a
		}

		name := t.Column.Name()
		if name == "" {
			a.bad = "ordered aggregate ORDER BY has an empty column"
			a.ordered = &orderedArgs{}

			return a
		}

		refs = append(refs, orderRef{column: name, desc: t.Desc})
	}

	a.ordered = &orderedArgs{terms: refs}

	return a
}

// aggregatable is the sealed set of inputs GroupedQuery.Agg accepts: a plain
// Aggregate or an OrderedAggregate[T]. Its method is unexported, so only
// orm's own aggregates can be passed.
type aggregatable interface{ aggregate() Aggregate }

// aggregate reports a's erased Aggregate. It is the seal that lets Agg accept
// both Aggregate and OrderedAggregate[T] without changing every existing call.
func (a Aggregate) aggregate() Aggregate { return a }

// groupKind discriminates a groupTerm's shape.
type groupKind int

// Supported group-term kinds.
const (
	groupPlain groupKind = iota
	groupRollup
	groupCube
	groupGroupingSets
)

// groupTerm is the erased shape of one GROUP BY element: a plain column or
// scalar expression (column/fn set), a ROLLUP/CUBE over terms, or a GROUPING
// SETS list of sets. The T type parameter is phantom ([0]T) -- it exists
// only so the groupable[T] interface ties a term back to its entity, keeping
// GroupBy(widgets.Column) from compiling against a different entity.
type groupTerm[T any] struct {
	_ [0]T

	kind   groupKind
	column string
	fn     *FuncExpr

	terms []groupTerm[T]
	sets  [][]groupTerm[T]
}

// groupable is the sealed set of GROUP BY elements: AnyColumn (a plain
// column), Expr (a scalar expression), and Grouping (a ROLLUP/CUBE/GROUPING
// SETS construct). Its method is unexported, so only orm's own types
// implement it -- callers pass values, never raw strings.
type groupable[T any] interface {
	grouping() groupTerm[T]
}

// grouping reports a's erased group term.
func (a AnyColumn[T]) grouping() groupTerm[T] {
	return groupTerm[T]{kind: groupPlain, column: a.Name()}
}

// grouping reports e's erased group term: a plain column when e is a bare
// Column.Expr() leaf, otherwise a scalar expression.
func (e Expr[T, V]) grouping() groupTerm[T] {
	if e.n.Kind == NColumn {
		return groupTerm[T]{kind: groupPlain, column: e.n.Column}
	}

	return groupTerm[T]{kind: groupPlain, fn: e.n.Func}
}

// GroupingSet is one parenthesized GROUPING SETS member -- a set of grouping
// elements combined as one grouping. Build it with NewGroupingSet; its
// fields are unexported so callers cannot forge grouping structure.
type GroupingSet[T any] struct{ terms []groupTerm[T] }

// NewGroupingSet builds one GROUPING SETS member from terms (columns and/or
// scalar expressions). The zero-term set `NewGroupingSet[T]()` is the empty
// set -- SQL's grand total `()`.
func NewGroupingSet[T any](terms ...groupable[T]) GroupingSet[T] {
	return GroupingSet[T]{terms: groupTerms(terms)}
}

// Grouping is a typed GROUP BY construct -- ROLLUP, CUBE or GROUPING SETS --
// built by Rollup/Cube/GroupingSets and accepted anywhere a GroupBy element
// is (including inside Rollup/Cube/NewGroupingSet).
type Grouping[T any] struct{ term groupTerm[T] }

// grouping reports g's erased group term.
func (g Grouping[T]) grouping() groupTerm[T] { return g.term }

// Rollup builds a `GROUP BY ROLLUP(...)` grouping over terms: the subset
// totals for every prefix of the argument list, plus the grand total. It is
// native on Postgres; MySQL renders its suffix form (`GROUP BY ... WITH
// ROLLUP`), which can only roll up the entire group list, so a MySQL ROLLUP
// combined with other group terms is a typed dialect.ErrUnsupportedByDialect;
// SQLite has no ROLLUP at any version and is likewise a typed error.
func Rollup[T any](terms ...groupable[T]) Grouping[T] {
	return Grouping[T]{term: groupTerm[T]{kind: groupRollup, terms: groupTerms(terms)}}
}

// Cube builds a `GROUP BY CUBE(...)` grouping over terms: the subtotals for
// every combination of the argument list, plus the grand total. Postgres
// only; MySQL and SQLite return a typed dialect.ErrUnsupportedByDialect.
func Cube[T any](terms ...groupable[T]) Grouping[T] {
	return Grouping[T]{term: groupTerm[T]{kind: groupCube, terms: groupTerms(terms)}}
}

// GroupingSets builds a `GROUP BY GROUPING SETS (...)` grouping from sets,
// each built by NewGroupingSet. Postgres only; MySQL and SQLite return a
// typed dialect.ErrUnsupportedByDialect.
func GroupingSets[T any](sets ...GroupingSet[T]) Grouping[T] {
	out := make([][]groupTerm[T], len(sets))

	for i, s := range sets {
		out[i] = s.terms
	}

	return Grouping[T]{term: groupTerm[T]{kind: groupGroupingSets, sets: out}}
}

// groupTerms erases groupable values into group terms.
func groupTerms[T any](terms []groupable[T]) []groupTerm[T] {
	out := make([]groupTerm[T], len(terms))

	for i, t := range terms {
		out[i] = t.grouping()
	}

	return out
}

// havingLeafKind/havingNode: HAVING's own tiny predicate tree.
//
// HAVING filters on aggregate EXPRESSIONS (`HAVING COUNT(*) > 5`) and on
// scalar expressions over grouped columns (`HAVING LOWER(category) = ?`),
// not real columns -- structurally different from Predicate[T]/Node, which
// is built from Column[T,V] values referring to a real table column. A small
// dedicated type here is the narrower change: it reuses Op/CompoundOp
// (already declared in predicate.go) for the comparison/combinator
// vocabulary, but keeps its own leaf shapes (an Aggregate for havingLeaf, an
// erased predicate Node for havingExpr).
type havingKind int

// Supported HAVING node kinds.
const (
	havingNone havingKind = iota
	havingLeaf
	havingCompound
	havingExpr
)

type havingNode struct {
	kind     havingKind
	agg      Aggregate
	op       Op
	value    any
	compound CompoundOp
	children []havingNode
	expr     Node
}

// HavingPredicate is a typed-as-far-as-erasure-allows HAVING predicate,
// built from an Aggregate's Eq/Neq/Gt/Gte/Lt/Lte comparison methods and
// combined with HavingAnd/HavingOr. Its zero value is "unset", mirroring
// Predicate[T]'s IsSet/Render contract.
//
// Type-safety tradeoff: Aggregate.Gt/Gte/Lt/Lte/Eq/Neq below accept `v any`,
// not a V pinned to the aggregate's original column type -- because
// Aggregate itself has already erased V by construction time (see
// Aggregate's doc comment). This is NOT a general "give up on typing"
// weakening: it's a compile-time guarantee genuinely impossible to give
// without redesigning Aggregate to carry a type parameter (which would
// break the "one []Aggregate slice, many column types" requirement every
// GroupBy(...).Agg(...) call relies on). The narrowest available
// mitigation is applied instead: orm/render.GroupedSelect performs a
// runtime type check (numeric-producing aggregates -- COUNT/SUM/AVG --
// reject an obviously non-numeric Value) and returns a rendering-time
// error, never a panic, on mismatch. MIN/MAX get no such check since they
// are legitimately comparable against any orderable type.
type HavingPredicate struct{ n havingNode }

// IsSet reports whether p carries an actual HAVING predicate.
func (p HavingPredicate) IsSet() bool { return p.n.kind != havingNone }

// havingArg is the sealed set of HAVING inputs accepted by
// GroupedQuery.Having: an aggregate comparison (HavingPredicate) or a scalar
// expression predicate (Predicate[T], e.g. Lower(col).Eq("x")).
type havingArg interface{ havingClause() havingNode }

// havingClause reports p's erased HAVING node.
func (p HavingPredicate) havingClause() havingNode { return p.n }

// havingClause reports the scalar predicate p as a HAVING expression leaf.
func (p Predicate[T]) havingClause() havingNode {
	return havingNode{kind: havingExpr, expr: p.n}
}

func (a Aggregate) compare(op Op, v any) HavingPredicate {
	return HavingPredicate{n: havingNode{kind: havingLeaf, agg: a, op: op, value: v}}
}

// Eq builds a `<agg> = v` HAVING predicate.
func (a Aggregate) Eq(v any) HavingPredicate { return a.compare(Eq, v) }

// Neq builds a `<agg> != v` HAVING predicate.
func (a Aggregate) Neq(v any) HavingPredicate { return a.compare(Neq, v) }

// Gt builds a `<agg> > v` HAVING predicate.
func (a Aggregate) Gt(v any) HavingPredicate { return a.compare(Gt, v) }

// Gte builds a `<agg> >= v` HAVING predicate.
func (a Aggregate) Gte(v any) HavingPredicate { return a.compare(Gte, v) }

// Lt builds a `<agg> < v` HAVING predicate.
func (a Aggregate) Lt(v any) HavingPredicate { return a.compare(Lt, v) }

// Lte builds a `<agg> <= v` HAVING predicate.
func (a Aggregate) Lte(v any) HavingPredicate { return a.compare(Lte, v) }

// HavingAnd combines ps with logical AND; see orm.And for the zero/
// one-element rules this mirrors exactly.
//
// Named HavingAnd/HavingOr rather than a second package-level And/Or
// overload: Go has no function overloading, generic or otherwise, so a
// non-generic `func And(ps ...HavingPredicate) HavingPredicate` cannot
// coexist with predicate.go's existing `func And[T any](ps
// ...Predicate[T]) Predicate[T]` under the same name.
func HavingAnd(ps ...HavingPredicate) HavingPredicate { return combineHaving(CAnd, ps) }

// HavingOr combines ps with logical OR; see HavingAnd.
func HavingOr(ps ...HavingPredicate) HavingPredicate { return combineHaving(COr, ps) }

// HavingNot negates p. Not of an unset predicate is itself unset.
func HavingNot(p HavingPredicate) HavingPredicate {
	if !p.IsSet() {
		return HavingPredicate{}
	}

	return HavingPredicate{n: havingNode{kind: havingCompound, compound: CNot, children: []havingNode{p.n}}}
}

func combineHaving(op CompoundOp, ps []HavingPredicate) HavingPredicate {
	children := make([]havingNode, 0, len(ps))

	for _, p := range ps {
		if p.IsSet() {
			children = append(children, p.n)
		}
	}

	switch len(children) {
	case 0:
		return HavingPredicate{}
	case 1:
		return HavingPredicate{n: children[0]}
	default:
		return HavingPredicate{n: havingNode{kind: havingCompound, compound: op, children: children}}
	}
}

func toRenderAggregate(a Aggregate) render.Aggregate {
	out := render.Aggregate{
		Func:         a.Func,
		Column:       a.Column,
		Alias:        a.Alias,
		DistinctArg:  a.distinct,
		Delimiter:    a.delim,
		HasDelimiter: a.hasDelim,
		Invalid:      a.bad,
	}

	if a.filter != nil {
		f := toRenderNodeErased(*a.filter)
		out.Filter = &f
	}

	if a.ordered != nil {
		out.Order = make([]render.OrderRef, len(a.ordered.terms))
		for i, t := range a.ordered.terms {
			out.Order[i] = render.OrderRef{Column: t.column, Desc: t.desc}
		}
	}

	return out
}

func toRenderAggregates(aggs []Aggregate) []render.Aggregate {
	out := make([]render.Aggregate, len(aggs))
	for i, a := range aggs {
		out[i] = toRenderAggregate(a)
	}

	return out
}

// toRenderNodeErased converts a orm.Node into render.Node. Its generic
// parent toRenderNode[T] only threads T through the scalar-expression
// conversion without ever producing a T-typed value, so the erased
// conversion is safe and lets the aggregate/HAVING converters stay
// non-generic.
func toRenderNodeErased(n Node) render.Node { return toRenderNode[struct{}](n) }

func toRenderHaving(n havingNode) render.HavingNode {
	switch n.kind {
	case havingLeaf:
		return render.HavingNode{Kind: render.HavingLeaf, Agg: toRenderAggregate(n.agg), Op: n.op, Value: n.value}
	case havingCompound:
		children := make([]render.HavingNode, len(n.children))
		for i, c := range n.children {
			children[i] = toRenderHaving(c)
		}

		return render.HavingNode{Kind: render.HavingCompound, Compound: n.compound, Children: children}
	case havingExpr:
		e := toRenderNodeErased(n.expr)

		return render.HavingNode{Kind: render.HavingExpr, Expr: &e}
	case havingNone:
		return render.HavingNode{}
	default:
		return render.HavingNode{}
	}
}

// toRenderGroups converts a group-term list into render's erased shape.
func toRenderGroups[T any](gs []groupTerm[T]) []render.GroupTerm {
	out := make([]render.GroupTerm, len(gs))

	for i, g := range gs {
		out[i] = toRenderGroupTerm(g)
	}

	return out
}

func toRenderGroupTerm[T any](g groupTerm[T]) render.GroupTerm {
	out := render.GroupTerm{Kind: render.GroupKind(g.kind), Column: g.column}

	if g.fn != nil {
		out.Func = toRenderFunc[T](g.fn)
	}

	if len(g.terms) > 0 {
		out.Terms = toRenderGroups(g.terms)
	}

	if len(g.sets) > 0 {
		out.Sets = make([][]render.GroupTerm, len(g.sets))
		for i, s := range g.sets {
			out.Sets[i] = toRenderGroups(s)
		}
	}

	return out
}

// GroupedQuery is an immutable, value-type GROUP BY/aggregate SELECT
// builder over entity T, built from a Query[T,PT] via GroupBy. It follows
// the same copy-on-write chain discipline as Query[T,PT] (see orm/query.go)
// -- Agg/Having each return a NEW GroupedQuery rather than mutating the
// receiver, and every slice-append copies into a fresh backing array
// first.
//
// GroupedQuery.Scan takes a callback rather than returning a typed
// []PT the way Query[T,PT].All does: an aggregate result row (group
// columns + aggregate values) does not correspond to any single T -- there
// is no codegen'd Scanner for an arbitrary GroupBy/Agg combination -- so a
// callback over the raw Row is the only shape that doesn't require
// generating a bespoke result struct per call site. This mirrors the legacy
// ORM backend's generated GroupedQuery.Scan(ctx, exec, fn func(db.Rows)
// error) exactly, which chose the same tradeoff for the same reason.
type GroupedQuery[T any] struct {
	table Table[T]
	where Predicate[T]
	// groupCols is the plain-column subset of groups, retained for the CTE
	// wrapper (WithGrouped), whose outer projection references CTE columns
	// by name and so cannot express grouping constructs or bare
	// expressions. groups is the full GROUP BY specification.
	groupCols []AnyColumn[T]
	groups    []groupTerm[T]
	aggs      []Aggregate
	having    HavingPredicate
}

// GroupBy starts a GroupedQuery over cols, carrying over q's table and
// WHERE filter (order/limit/offset are meaningless for a grouped aggregate
// and are dropped). Elements may be plain AnyColumn refs (Column.Col()), a
// scalar Expr[T,V], or a Rollup/Cube/GroupingSets construct.
func (q Query[T, PT]) GroupBy(cols ...groupable[T]) GroupedQuery[T] {
	next := make([]groupTerm[T], len(cols))
	plain := make([]AnyColumn[T], 0, len(cols))

	for i, c := range cols {
		term := c.grouping()
		next[i] = term

		if term.kind == groupPlain && term.fn == nil && term.column != "" {
			plain = append(plain, AnyColumn[T]{name: term.column})
		}
	}

	return GroupedQuery[T]{table: q.table, where: q.where, groupCols: plain, groups: next}
}

// hasAdvancedGroups reports whether g uses a GROUP BY element the CTE
// wrapper cannot project by column name -- a scalar expression or a
// grouping construct.
func (g GroupedQuery[T]) hasAdvancedGroups() bool {
	for _, term := range g.groups {
		if term.kind != groupPlain || term.fn != nil {
			return true
		}
	}

	return false
}

// Agg appends aggregate expressions to g's select list. It copies g.aggs
// into a FRESH backing array before appending, matching Query[T,PT].
// OrderBy's branch-safety rule.
func (g GroupedQuery[T]) Agg(aggs ...aggregatable) GroupedQuery[T] {
	next := make([]Aggregate, 0, len(g.aggs)+len(aggs))
	next = append(next, g.aggs...)

	for _, a := range aggs {
		next = append(next, a.aggregate())
	}

	g.aggs = next

	return g
}

// Having combines the given HAVING clauses into g's existing HAVING filter
// with AND, mirroring Query[T,PT].Where's rule exactly (the receiver is left
// unmodified). Each argument is either an aggregate comparison
// (HavingPredicate from an Aggregate's Gt/Eq/... method) or a scalar
// expression predicate (Predicate[T], e.g. Lower(col).Eq("x")); the two
// forms compose freely and preserve WHERE-then-HAVING argument order.
func (g GroupedQuery[T]) Having(args ...havingArg) GroupedQuery[T] {
	for _, arg := range args {
		n := arg.havingClause()
		if n.kind == havingNone {
			continue
		}

		if g.having.IsSet() {
			g.having = HavingAnd(g.having, HavingPredicate{n: n})
		} else {
			g.having = HavingPredicate{n: n}
		}
	}

	return g
}

// Scan runs g against exec and hands every result row to fn. Result
// columns are g's GroupBy columns in order, followed by each aggregate in
// the order Agg was called -- see GroupedQuery's doc comment for why this
// is a callback rather than a typed materializing method.
func (g GroupedQuery[T]) Scan(ctx context.Context, exec db.DB, fn func(row Row) error) error {
	d, err := resolveDialect(exec)
	if err != nil {
		return err
	}

	query, args, err := render.GroupedSelect(
		d,
		g.table.Name(),
		toRenderGroups(g.groups),
		toRenderAggregates(g.aggs),
		toRenderNode[T](g.where.Render()),
		toRenderHaving(g.having.n),
	)
	if err != nil {
		return fmt.Errorf("orm: GroupedQuery.Scan: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return fmt.Errorf("orm: GroupedQuery.Scan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := fn(rows); err != nil {
			_ = rows.Close()

			return fmt.Errorf("orm: GroupedQuery.Scan: row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: GroupedQuery.Scan: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("orm: GroupedQuery.Scan: %w", err)
	}

	return nil
}
