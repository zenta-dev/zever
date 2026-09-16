package orm

import "strings"

// Column is a typed reference to one non-nullable column of entity T
// whose SQL value type is V. Every comparison method only accepts a V, so
// e.g. a Column[User, string]'s Eq only compiles against a string --
// caught at compile time, not by a renderer type-asserting an `any` at run
// time.
//
// Column's fields are unexported: the only way to construct a working
// Column is NewColumn, which schema codegen calls with schema-derived
// table/column names. A hand-written Column[User,string]{} outside this
// package is a useless zero value, never a working forged column -- the
// concrete mechanism behind "codegen only, never a caller string".
type Column[T any, V any] struct {
	table string
	name  string
}

// NewColumn builds a Column bound to table/name. It exists for schema
// codegen to call; hand-writing a call to it outside generated code
// defeats the purpose of Column's unexported fields and should be avoided.
func NewColumn[T any, V any](table, name string) Column[T, V] {
	return Column[T, V]{table: table, name: name}
}

func (c Column[T, V]) simple(op Op, value any) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: value}}
}

// Eq builds a Column = v predicate. See ExampleColumn_Eq for a runnable
// example.
func (c Column[T, V]) Eq(v V) Predicate[T] { return c.simple(Eq, v) }

// Neq builds a Column != v predicate.
func (c Column[T, V]) Neq(v V) Predicate[T] { return c.simple(Neq, v) }

// Gt builds a Column > v predicate.
func (c Column[T, V]) Gt(v V) Predicate[T] { return c.simple(Gt, v) }

// Gte builds a Column >= v predicate.
func (c Column[T, V]) Gte(v V) Predicate[T] { return c.simple(Gte, v) }

// Lt builds a Column < v predicate.
func (c Column[T, V]) Lt(v V) Predicate[T] { return c.simple(Lt, v) }

// Lte builds a Column <= v predicate.
func (c Column[T, V]) Lte(v V) Predicate[T] { return c.simple(Lte, v) }

// EqOuter builds a `Column = <outer column>` predicate: a correlated
// comparison against a column of the ENCLOSING query, spelled with the
// explicit Outer suffix because Go cannot overload Eq on the value type
// (the RHS materializes as an OuterRef marker, never a V). Meaningful only
// inside a subquery predicate's Where (Exists/NotExists/in-subquery/scalar):
// the renderer binds the marker to the directly enclosing single-table
// SELECT at execution time, rendering the outer column fully qualified. Any
// other placement is a typed rendering error, never wrong SQL (see Outer).
func (c Column[T, V]) EqOuter[U any](outer OuterRef[U, V]) Predicate[T] { return c.outerCmp(Eq, outer) }

// NeqOuter builds a `Column != <outer column>` predicate; see EqOuter.
func (c Column[T, V]) NeqOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Neq, outer)
}

// GtOuter builds a `Column > <outer column>` predicate; see EqOuter.
func (c Column[T, V]) GtOuter[U any](outer OuterRef[U, V]) Predicate[T] { return c.outerCmp(Gt, outer) }

// GteOuter builds a `Column >= <outer column>` predicate; see EqOuter.
func (c Column[T, V]) GteOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Gte, outer)
}

// LtOuter builds a `Column < <outer column>` predicate; see EqOuter.
func (c Column[T, V]) LtOuter[U any](outer OuterRef[U, V]) Predicate[T] { return c.outerCmp(Lt, outer) }

// LteOuter builds a `Column <= <outer column>` predicate; see EqOuter.
func (c Column[T, V]) LteOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Lte, outer)
}

// outerCmp builds a KindBinary comparison whose Value is the OuterRef marker
// untouched -- the marker (not a bound value) is what tells the renderer the
// RHS is a correlated column reference, resolved against the enclosing query
// at execution time.
func (c Column[T, V]) outerCmp[U any](op Op, outer OuterRef[U, V]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: outer}}
}

// In builds a Column IN (vs...) predicate. vs is copied into a []any leaf
// through a plain typed loop -- never reflect.
//
// In is a pure AST builder -- it has no query-executor context to chunk
// against, so a very large vs produces a single IN (...) with as many bound
// placeholders as len(vs), risking driver/DB placeholder limits (Postgres
// ~65535 params, SQLite ~999-32766 depending on build flags) or a
// degenerate query plan. Callers with a very large id set should chunk it
// themselves before calling In repeatedly across multiple queries and
// combining the results.
func (c Column[T, V]) In(vs ...V) Predicate[T] {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return Predicate[T]{n: Node{Kind: NIn, Table: c.table, Column: c.name, Op: In, Value: out}}
}

// InSub builds a Column IN (<inner SELECT>) predicate -- `col IN (SELECT
// ...)`. inner is a query over any entity C; the subquery form cannot reuse
// In(...V) because Go generics forbid overloading a method on its value
// type, so the subquery shape is spelled InSub instead. inner must project
// exactly one column (via Query.Columns); a renderer enforces that with a
// typed error at execution time rather than shipping multi-column IN SQL.
func (c Column[T, V]) InSub[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.inSub(In, inner)
}

// NotInSub builds a Column NOT IN (<inner SELECT>) predicate. Unlike
// Not(c.InSub(...)), which renders an outer `NOT (col IN (SELECT ...))`
// wrapper, NotInSub carries the negation INTO the clause and renders
// `col NOT IN (SELECT ...)`. inner must project exactly one column; see
// InSub.
func (c Column[T, V]) NotInSub[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.inSub(NotIn, inner)
}

func (c Column[T, V]) inSub[C any, PC ptrScanner[C]](op Op, inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NIn, Table: c.table, Column: c.name, Op: op, Value: toSubquery(inner)}}
}

// EqScalar builds a Column = (<inner SELECT>) predicate: the column is
// compared against the single cell a one-row, one-column inner query
// returns. inner must project exactly one column (via Query.Columns), or
// rendering fails with a typed error. Go cannot overload Eq, so the scalar
// comparisons are spelled with a Scalar suffix.
func (c Column[T, V]) EqScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Eq, inner)
}

// NeqScalar builds a Column != (<inner SELECT>) predicate; see EqScalar.
func (c Column[T, V]) NeqScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Neq, inner)
}

// GtScalar builds a Column > (<inner SELECT>) predicate; see EqScalar.
func (c Column[T, V]) GtScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Gt, inner)
}

// GteScalar builds a Column >= (<inner SELECT>) predicate; see EqScalar.
func (c Column[T, V]) GteScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Gte, inner)
}

// LtScalar builds a Column < (<inner SELECT>) predicate; see EqScalar.
func (c Column[T, V]) LtScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Lt, inner)
}

// LteScalar builds a Column <= (<inner SELECT>) predicate; see EqScalar.
func (c Column[T, V]) LteScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Lte, inner)
}

func (c Column[T, V]) scalarSub[C any, PC ptrScanner[C]](op Op, inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: toSubquery(inner)}}
}

// Between builds a Column BETWEEN lo AND hi predicate.
func (c Column[T, V]) Between(lo, hi V) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBetween, Table: c.table, Column: c.name, Op: Between, Value: [2]any{lo, hi}}}
}

// EqAny builds a Postgres `Column = ANY(values...)` predicate: true when the
// column equals at least one of values. It renders as
// `"col" = ANY(ARRAY[$1, $2, ...])` with one bound placeholder per element
// (see render/array.go); an empty values list renders an always-false clause
// rather than invalid SQL. Postgres-only: other dialects return a typed
// dialect.ErrUnsupportedByDialect at execution time, never silently-wrong
// SQL.
func (c Column[T, V]) EqAny(vs ...V) Predicate[T] { return c.array(EqAny, vs) }

// NeqAny builds a Postgres `Column <> ANY(values...)` predicate: true when
// the column differs from at least one of values. See EqAny.
func (c Column[T, V]) NeqAny(vs ...V) Predicate[T] { return c.array(NeqAny, vs) }

// EqAll builds a Postgres `Column = ALL(values...)` predicate: true when the
// column equals every element of values. See EqAny.
func (c Column[T, V]) EqAll(vs ...V) Predicate[T] { return c.array(EqAll, vs) }

// NeqAll builds a Postgres `Column <> ALL(values...)` predicate: true when
// the column differs from every element of values -- the array-quantifier
// spelling of NOT IN that is also true for the empty list. See EqAny.
func (c Column[T, V]) NeqAll(vs ...V) Predicate[T] { return c.array(NeqAll, vs) }

// array builds an NArray predicate whose Value is the bound element list,
// copied through a plain typed loop -- never reflect, the same convention
// In uses.
func (c Column[T, V]) array(op Op, vs []V) Predicate[T] {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return Predicate[T]{n: Node{Kind: NArray, Table: c.table, Column: c.name, Op: op, Value: out}}
}

// Asc builds an ascending OrderTerm for c.
func (c Column[T, V]) Asc() OrderTerm[T] { return OrderTerm[T]{Column: c.Col()} }

// Desc builds a descending OrderTerm for c.
func (c Column[T, V]) Desc() OrderTerm[T] { return OrderTerm[T]{Column: c.Col(), Desc: true} }

// Col erases c's value type, for column lists where V genuinely doesn't
// matter: Query.Columns projections (used by subquery operands) and the
// `FOR UPDATE OF` table list.
func (c Column[T, V]) Col() AnyColumn[T] { return AnyColumn[T]{name: c.name, table: c.table} }

// Table returns c's SQL table name. Exported for expression builders to
// reference the column's table when they construct predicate nodes that
// must render qualified in joins.
func (c Column[T, V]) Table() string { return c.table }

// Name returns c's SQL column name.
func (c Column[T, V]) Name() string { return c.name }

// escapeLike escapes s for safe use inside a LIKE pattern built by
// Contains/StartsWith/EndsWith: a literal backslash, percent or underscore
// in s must never be interpreted as a LIKE wildcard. Order matters --
// backslash must be escaped first, or the escaping added for % and _ would
// itself be re-escaped.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

	return r.Replace(s)
}

// Contains builds a Column LIKE '%s%' predicate over a string-valued
// Column, with s's own %, _ and \ characters escaped so they match
// literally rather than acting as LIKE wildcards. render always appends
// ESCAPE '\' to a Like clause, so this escaping is meaningful on every
// supported dialect.
//
// This is a free function, not a Column[T, string] method: Go's generic
// receiver syntax only accepts type-parameter NAMES in a receiver type
// parameter list, not a concrete type argument like "string".
func Contains[T any](c Column[T, string], s string) Predicate[T] {
	return c.simple(Like, "%"+escapeLike(s)+"%").asLike()
}

// StartsWith builds a Column LIKE 's%' predicate; see Contains for escaping
// and why this is a free function rather than a method.
func StartsWith[T any](c Column[T, string], s string) Predicate[T] {
	return c.simple(Like, escapeLike(s)+"%").asLike()
}

// EndsWith builds a Column LIKE '%s' predicate; see Contains for escaping
// details.
func EndsWith[T any](c Column[T, string], s string) Predicate[T] {
	return c.simple(Like, "%"+escapeLike(s)).asLike()
}

// LikeRaw builds a Column LIKE pattern predicate with NO escaping of
// pattern's own %/_/\ characters -- the explicit opt-in unescaped case
// (Contains/StartsWith/EndsWith are the safe default).
func LikeRaw[T any](c Column[T, string], pattern string) Predicate[T] {
	return c.simple(Like, pattern).asLike()
}

// asLike re-tags a Node built via simple(Like, ...) with NLike's Kind, since
// simple always tags NBinary.
func (p Predicate[T]) asLike() Predicate[T] {
	p.n.Kind = NLike

	return p
}

// NullableColumn is a typed reference to a column of entity T that
// may hold SQL NULL, whose non-NULL value type is V. Unlike Column[T,V], it
// deliberately has no Eq(nil)-shaped overload -- every comparison method
// below only accepts a bare V, so "compare to NULL" is only ever spellable
// as IsNull()/IsNotNull(), never a silently-wrong `col = NULL`. It carries
// no relationship to Option[T] at all: Option is the scan-side
// representation of a nullable value, while NullableColumn builds
// predicates.
type NullableColumn[T any, V any] struct {
	table string
	name  string
}

// NewNullableColumn builds a NullableColumn bound to table/name, for
// schema codegen to call.
func NewNullableColumn[T any, V any](table, name string) NullableColumn[T, V] {
	return NullableColumn[T, V]{table: table, name: name}
}

func (c NullableColumn[T, V]) simple(op Op, value any) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: value}}
}

// Eq builds a Column = v predicate.
func (c NullableColumn[T, V]) Eq(v V) Predicate[T] { return c.simple(Eq, v) }

// Neq builds a Column != v predicate.
func (c NullableColumn[T, V]) Neq(v V) Predicate[T] { return c.simple(Neq, v) }

// Gt builds a Column > v predicate.
func (c NullableColumn[T, V]) Gt(v V) Predicate[T] { return c.simple(Gt, v) }

// Gte builds a Column >= v predicate.
func (c NullableColumn[T, V]) Gte(v V) Predicate[T] { return c.simple(Gte, v) }

// Lt builds a Column < v predicate.
func (c NullableColumn[T, V]) Lt(v V) Predicate[T] { return c.simple(Lt, v) }

// Lte builds a Column <= v predicate.
func (c NullableColumn[T, V]) Lte(v V) Predicate[T] { return c.simple(Lte, v) }

// EqOuter builds a `Column = <outer column>` predicate: a correlated
// comparison against a column of the ENCLOSING query, spelled with the
// explicit Outer suffix because Go cannot overload Eq on the value type
// (the RHS materializes as an OuterRef marker, never a V). Meaningful only
// inside a subquery predicate's Where (Exists/NotExists/in-subquery/scalar):
// the renderer binds the marker to the directly enclosing single-table
// SELECT at execution time, rendering the outer column fully qualified. Any
// other placement is a typed rendering error, never wrong SQL (see Outer).
func (c NullableColumn[T, V]) EqOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Eq, outer)
}

// NeqOuter builds a `Column != <outer column>` predicate; see EqOuter.
func (c NullableColumn[T, V]) NeqOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Neq, outer)
}

// GtOuter builds a `Column > <outer column>` predicate; see EqOuter.
func (c NullableColumn[T, V]) GtOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Gt, outer)
}

// GteOuter builds a `Column >= <outer column>` predicate; see EqOuter.
func (c NullableColumn[T, V]) GteOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Gte, outer)
}

// LtOuter builds a `Column < <outer column>` predicate; see EqOuter.
func (c NullableColumn[T, V]) LtOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Lt, outer)
}

// LteOuter builds a `Column <= <outer column>` predicate; see EqOuter.
func (c NullableColumn[T, V]) LteOuter[U any](outer OuterRef[U, V]) Predicate[T] {
	return c.outerCmp(Lte, outer)
}

// outerCmp builds a KindBinary comparison whose Value is the OuterRef marker
// untouched -- the marker (not a bound value) is what tells the renderer the
// RHS is a correlated column reference, resolved against the enclosing query
// at execution time.
func (c NullableColumn[T, V]) outerCmp[U any](op Op, outer OuterRef[U, V]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: outer}}
}

// In builds a Column IN (vs...) predicate.
func (c NullableColumn[T, V]) In(vs ...V) Predicate[T] {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return Predicate[T]{n: Node{Kind: NIn, Table: c.table, Column: c.name, Op: In, Value: out}}
}

// InSub builds a Column IN (<inner SELECT>) predicate; inner must project
// exactly one column (via Query.Columns). See Column.InSub for the full
// contract.
func (c NullableColumn[T, V]) InSub[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.inSub(In, inner)
}

// NotInSub builds a Column NOT IN (<inner SELECT>) predicate, rendering
// `col NOT IN (SELECT ...)`; inner must project exactly one column. See
// Column.InSub.
func (c NullableColumn[T, V]) NotInSub[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.inSub(NotIn, inner)
}

func (c NullableColumn[T, V]) inSub[C any, PC ptrScanner[C]](op Op, inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NIn, Table: c.table, Column: c.name, Op: op, Value: toSubquery(inner)}}
}

// EqScalar builds a Column = (<inner SELECT>) predicate; inner must
// project exactly one column. See Column.EqScalar.
func (c NullableColumn[T, V]) EqScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Eq, inner)
}

// NeqScalar builds a Column != (<inner SELECT>) predicate; see EqScalar.
func (c NullableColumn[T, V]) NeqScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Neq, inner)
}

// GtScalar builds a Column > (<inner SELECT>) predicate; see EqScalar.
func (c NullableColumn[T, V]) GtScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Gt, inner)
}

// GteScalar builds a Column >= (<inner SELECT>) predicate; see EqScalar.
func (c NullableColumn[T, V]) GteScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Gte, inner)
}

// LtScalar builds a Column < (<inner SELECT>) predicate; see EqScalar.
func (c NullableColumn[T, V]) LtScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Lt, inner)
}

// LteScalar builds a Column <= (<inner SELECT>) predicate; see EqScalar.
func (c NullableColumn[T, V]) LteScalar[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return c.scalarSub(Lte, inner)
}

func (c NullableColumn[T, V]) scalarSub[C any, PC ptrScanner[C]](op Op, inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBinary, Table: c.table, Column: c.name, Op: op, Value: toSubquery(inner)}}
}

// Between builds a Column BETWEEN lo AND hi predicate.
func (c NullableColumn[T, V]) Between(lo, hi V) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NBetween, Table: c.table, Column: c.name, Op: Between, Value: [2]any{lo, hi}}}
}

// EqAny builds a Postgres `Column = ANY(values...)` predicate; see
// Column.EqAny for the surface, the Postgres-only gate and the empty-list
// rule.
func (c NullableColumn[T, V]) EqAny(vs ...V) Predicate[T] { return c.array(EqAny, vs) }

// NeqAny builds a Postgres `Column <> ANY(values...)` predicate; see
// Column.EqAny.
func (c NullableColumn[T, V]) NeqAny(vs ...V) Predicate[T] { return c.array(NeqAny, vs) }

// EqAll builds a Postgres `Column = ALL(values...)` predicate; see
// Column.EqAny.
func (c NullableColumn[T, V]) EqAll(vs ...V) Predicate[T] { return c.array(EqAll, vs) }

// NeqAll builds a Postgres `Column <> ALL(values...)` predicate; see
// Column.EqAny.
func (c NullableColumn[T, V]) NeqAll(vs ...V) Predicate[T] { return c.array(NeqAll, vs) }

func (c NullableColumn[T, V]) array(op Op, vs []V) Predicate[T] {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return Predicate[T]{n: Node{Kind: NArray, Table: c.table, Column: c.name, Op: op, Value: out}}
}

// IsNull builds a Column IS NULL predicate -- the only way to compare this
// column against NULL.
func (c NullableColumn[T, V]) IsNull() Predicate[T] { return c.simple(IsNull, nil) }

// IsNotNull builds a Column IS NOT NULL predicate.
func (c NullableColumn[T, V]) IsNotNull() Predicate[T] { return c.simple(IsNotNull, nil) }

// Asc builds an ascending OrderTerm for c.
func (c NullableColumn[T, V]) Asc() OrderTerm[T] { return OrderTerm[T]{Column: c.Col()} }

// Desc builds a descending OrderTerm for c.
func (c NullableColumn[T, V]) Desc() OrderTerm[T] { return OrderTerm[T]{Column: c.Col(), Desc: true} }

// Col erases c's value type, for column lists where V genuinely doesn't
// matter: Query.Columns projections (used by subquery operands) and the
// `FOR UPDATE OF` table list.
func (c NullableColumn[T, V]) Col() AnyColumn[T] { return AnyColumn[T]{name: c.name, table: c.table} }

// Table returns c's SQL table name. Exported for expression builders to
// reference the column's table when they construct predicate nodes that
// must render qualified in joins.
func (c NullableColumn[T, V]) Table() string { return c.table }

// Name returns c's SQL column name.
func (c NullableColumn[T, V]) Name() string { return c.name }

// Assignment is one column=value pair for an UPDATE/INSERT statement, built
// by the mutation builders or by NullableColumn.SetValue/SetNull. Column is
// an AnyColumn[T] -- never a raw string -- so a caller cannot inject SQL
// structure through an assignment's column.
type Assignment[T any] struct {
	Column AnyColumn[T]
	Value  any
}

// SetValue builds a non-NULL Assignment of c to v.
func (c NullableColumn[T, V]) SetValue(v V) Assignment[T] {
	return Assignment[T]{Column: c.Col(), Value: v}
}

// SetNull builds an explicit-NULL Assignment of c.
func (c NullableColumn[T, V]) SetNull() Assignment[T] {
	return Assignment[T]{Column: c.Col(), Value: nil}
}

// AnyColumn is a type-erased reference to a Column/NullableColumn's name and
// home table, used exactly where the value type genuinely doesn't matter --
// projection/grouping/set-operation column lists and the `FOR UPDATE OF`
// table list -- so those call sites stay column-ref-typed (not bare strings)
// without forcing every argument to share one V.
type AnyColumn[T any] struct {
	name  string
	table string
}

// Name returns the erased column name.
func (a AnyColumn[T]) Name() string { return a.name }

// Table returns the column's home table name, or "" when the reference was
// built without one. It is used to render the Postgres `FOR UPDATE OF`
// table list.
func (a AnyColumn[T]) Table() string { return a.table }
