package orm

// Expr is a typed scalar SQL expression over entity T producing SQL values
// of Go type V. It is the general scalar-expression surface the typed
// columns lack on their own: a nested tree of scalar functions and CASE
// conditionals built from columns, bound literals and other Expr values, e.g.
// COALESCE(bio, 'n/a'), LOWER(name), NULLIF(name, 'x') or
// CASE WHEN quantity >= 20 THEN 'big' ELSE 'small' END.
//
// An Expr is not itself a predicate: comparison methods (Eq/Neq/Gt/Gte/Lt/
// Lte, In, IsNull/IsNotNull) root it into a Predicate[T] that composes with
// And/Or/Not exactly like a Column's own comparisons, and Asc/Desc root it
// into an OrderTerm[T] so a result set can be ordered by the expression. The
// value type V is compile-time pinned, so e.g. a string-valued expression
// can never be compared against an int.
//
// Expr's fields are unexported and there is no exported constructor that
// accepts a raw function name, so the SQL structure of an expression is
// fixed by this package's constructors -- a caller controls only bound
// values, never identifiers or function names.
type Expr[T any, V any] struct{ n Node }

// Expr returns c as a scalar expression leaf, so a Column can be passed to
// the expression constructors (Coalesce, Lower, ...) and nested inside a
// larger expression. It is a no-op wrapper: the returned Expr renders as the
// same quoted column c does.
func (c Column[T, V]) Expr() Expr[T, V] {
	return Expr[T, V]{n: Node{Kind: NColumn, Table: c.table, Column: c.name}}
}

// Expr returns c as a scalar expression leaf; see Column.Expr. It exists so
// a NullableColumn can seed a COALESCE/NULLIF expression without losing its
// NULL-aware typing (the expression's result type is a bare V -- the
// fallback's job is precisely to remove the NULL).
func (c NullableColumn[T, V]) Expr() Expr[T, V] {
	return Expr[T, V]{n: Node{Kind: NColumn, Table: c.table, Column: c.name}}
}

// FuncExpr is the erased, render-ready shape of a scalar function or CASE
// expression tree: Function/Case's Name plus its ordered Args, or a CASE's
// WHEN arms and optional ELSE. Args elements are themselves Nodes using the
// NColumn (column leaf), NLit (bound literal) or NFunc (nested expression)
// kinds, so an expression nests to any depth.
//
// FuncExpr is exported only so render (which must not import orm) can
// consume the shape across the package boundary, mirroring JSONExpr/FTSExpr.
// Ordinary calling code builds expressions through this file's constructors
// and should never construct a FuncExpr directly.
type FuncExpr struct {
	// Name is the SQL function name for a plain function expression
	// (COALESCE/NULLIF/LOWER/UPPER/TRIM/LENGTH). Unused when Case is true.
	Name string
	// Args is a function's ordered argument list.
	Args []Node
	// Case reports that this is a CASE expression rather than a function
	// call: Whens carries its WHEN ... THEN ... arms and Else its optional
	// ELSE value.
	Case bool
	// Whens is a CASE's WHEN ... THEN ... arms, in order.
	Whens []FuncWhen
	// Else is a CASE's ELSE value expression, or nil for no ELSE.
	Else *Node
}

// FuncWhen is one CASE WHEN <Cond> THEN <Then> arm. Cond is a boolean
// predicate Node; Then a scalar expression Node (usually an NLit bound
// value).
type FuncWhen struct {
	Cond Node
	Then Node
}

// scalarLit wraps a bound value as an NLit scalar-expression leaf.
func scalarLit(v any) Node { return Node{Kind: NLit, Value: v} }

// scalarCall builds an NFunc expression node for a named function call.
func scalarCall[T any, V any](name string, args ...Node) Expr[T, V] {
	return Expr[T, V]{n: Node{Kind: NFunc, Func: &FuncExpr{Name: name, Args: args}}}
}

// Coalesce builds COALESCE(e, fallback): the first non-NULL of e and the
// bound fallback. COALESCE is standard SQL with identical semantics and
// syntax on Postgres and SQLite.
func Coalesce[T any, V any](e Expr[T, V], fallback V) Expr[T, V] {
	return scalarCall[T, V]("COALESCE", e.n, scalarLit(fallback))
}

// CoalesceNullable builds COALESCE(c, fallback) from a NullableColumn
// directly -- the ergonomic leaf form of Coalesce, mirroring the Column/
// NullableColumn split the comparison methods follow.
func CoalesceNullable[T any, V any](c NullableColumn[T, V], fallback V) Expr[T, V] {
	return Coalesce(c.Expr(), fallback)
}

// NullIf builds NULLIF(e, val): NULL when e equals the bound val, else e.
// NULLIF is standard SQL with identical semantics and syntax on Postgres
// and SQLite.
func NullIf[T any, V any](e Expr[T, V], val V) Expr[T, V] {
	return scalarCall[T, V]("NULLIF", e.n, scalarLit(val))
}

// Lower builds LOWER(e) over a string-valued expression. LOWER is standard
// SQL with identical semantics and syntax on Postgres and SQLite.
func Lower[T any](e Expr[T, string]) Expr[T, string] {
	return scalarCall[T, string]("LOWER", e.n)
}

// Upper builds UPPER(e) over a string-valued expression. UPPER is standard
// SQL with identical semantics and syntax on Postgres and SQLite.
func Upper[T any](e Expr[T, string]) Expr[T, string] {
	return scalarCall[T, string]("UPPER", e.n)
}

// Trim builds TRIM(e) over a string-valued expression, removing leading and
// trailing spaces. TRIM is standard SQL with identical semantics and syntax
// on Postgres and SQLite.
func Trim[T any](e Expr[T, string]) Expr[T, string] {
	return scalarCall[T, string]("TRIM", e.n)
}

// Length builds LENGTH(e): the character count of a string-valued
// expression. Both supported dialects accept LENGTH(e) with character
// semantics. The result binds as an int64.
func Length[T any](e Expr[T, string]) Expr[T, int64] {
	return scalarCall[T, int64]("LENGTH", e.n)
}

// compare roots e into a Predicate[T] comparing its result against the
// bound value v with op; the expression's own argument list renders BEFORE
// the comparison placeholder, so bound fallbacks/NULLIF values keep
// text-order placeholder numbering.
func (e Expr[T, V]) compare(op Op, v V) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NFunc, Func: e.n.Func, Op: op, Value: v}}
}

// simple roots e into a Predicate[T] with op and a raw (possibly nil or
// slice) value, for IsNull/IsNotNull/In.
func (e Expr[T, V]) simple(op Op, value any) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NFunc, Func: e.n.Func, Op: op, Value: value}}
}

// Eq builds an `expr = v` predicate.
func (e Expr[T, V]) Eq(v V) Predicate[T] { return e.compare(Eq, v) }

// Neq builds an `expr != v` predicate.
func (e Expr[T, V]) Neq(v V) Predicate[T] { return e.compare(Neq, v) }

// Gt builds an `expr > v` predicate.
func (e Expr[T, V]) Gt(v V) Predicate[T] { return e.compare(Gt, v) }

// Gte builds an `expr >= v` predicate.
func (e Expr[T, V]) Gte(v V) Predicate[T] { return e.compare(Gte, v) }

// Lt builds an `expr < v` predicate.
func (e Expr[T, V]) Lt(v V) Predicate[T] { return e.compare(Lt, v) }

// Lte builds an `expr <= v` predicate.
func (e Expr[T, V]) Lte(v V) Predicate[T] { return e.compare(Lte, v) }

// In builds an `expr IN (vs...)` predicate.
func (e Expr[T, V]) In(vs ...V) Predicate[T] {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return e.simple(In, out)
}

// IsNull builds an `expr IS NULL` predicate.
func (e Expr[T, V]) IsNull() Predicate[T] { return e.simple(IsNull, nil) }

// IsNotNull builds an `expr IS NOT NULL` predicate.
func (e Expr[T, V]) IsNotNull() Predicate[T] { return e.simple(IsNotNull, nil) }

// Asc builds an ascending OrderTerm ordering by the expression.
func (e Expr[T, V]) Asc() OrderTerm[T] { return OrderTerm[T]{Func: e.n.Func} }

// Desc builds a descending OrderTerm ordering by the expression.
func (e Expr[T, V]) Desc() OrderTerm[T] { return OrderTerm[T]{Func: e.n.Func, Desc: true} }

// CaseBuilder is the empty start of a searched CASE expression, returned by
// Case. Its When method yields an OngoingCase; only OngoingCase (which is
// guaranteed to hold at least one WHEN arm) can be finished with Else, so a
// syntactically invalid `CASE ELSE ... END` is unrepresentable.
type CaseBuilder[T any, V any] struct{}

// OngoingCase is a CASE under construction with one or more WHEN arms. It is
// immutable like every other orm builder: When returns a new value rather
// than mutating the receiver, so a partially built CASE can be branched
// safely.
type OngoingCase[T any, V any] struct{ whens []FuncWhen }

// Case starts a typed searched CASE expression over entity T whose result
// type is V: a chain of When arms finished by Else, e.g.
//
//	orm.Case[widget, string]().
//		When(widgetQty.Gte(20), "big").
//		Else("small")
//
// Both type arguments must be given at the Case call site (Go cannot infer
// them from the later method calls): T is the entity whose Predicate roots
// the WHEN conditions, V the THEN/ELSE value type.
func Case[T any, V any]() CaseBuilder[T, V] { return CaseBuilder[T, V]{} }

// When adds the first WHEN arm: WHEN p THEN val.
func (CaseBuilder[T, V]) When(p Predicate[T], val V) OngoingCase[T, V] {
	return OngoingCase[T, V]{whens: []FuncWhen{{Cond: p.Render(), Then: scalarLit(val)}}}
}

// When appends a WHEN arm: WHEN p THEN val.
func (o OngoingCase[T, V]) When(p Predicate[T], val V) OngoingCase[T, V] {
	next := make([]FuncWhen, 0, len(o.whens)+1)
	next = append(next, o.whens...)
	next = append(next, FuncWhen{Cond: p.Render(), Then: scalarLit(val)})
	o.whens = next

	return o
}

// Else finishes the CASE with `ELSE val END`, returning the expression. The
// receiver is left unmodified.
func (o OngoingCase[T, V]) Else(val V) Expr[T, V] {
	elseNode := scalarLit(val)

	return Expr[T, V]{n: Node{Kind: NFunc, Func: &FuncExpr{Case: true, Whens: o.whens, Else: &elseNode}}}
}
