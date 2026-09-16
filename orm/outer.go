package orm

// OuterRef is the typed marker for a correlated reference to one column of
// the ENCLOSING query, built by Outer/OuterNullable from a codegen'd
// Column/NullableColumn and accepted by a subquery-operand column's
// EqOuter/NeqOuter/... comparison methods. It keeps the outer column's VALUE
// type V, so the inner column's comparison only compiles against an outer
// reference of the same SQL value type (this column's EqOuter also names the
// outer ENTITY as a type parameter, so a reference to the wrong entity's
// column type-checks to nothing).
//
// OuterRef carries NO SQL text and NO column-qualifier names: it holds the
// outer column's home table and column name, and the renderer validates that
// table against the ACTUAL enclosing statement's table set at execution time
// (see render.OuterRef and render's scope handling) before qualifying the
// column with it, so a construction-time snapshot can never freeze the wrong
// outer table and a marker outside the enclosing set fails closed.
//
// OuterRef's fields are unexported: the only way to construct one is
// Outer/OuterNullable, which accept a codegen-derived Column/NullableColumn
// -- never a caller string.
type OuterRef[T any, V any] struct {
	table string
	name  string
}

func (r OuterRef[T, V]) outerRefTable() string { return r.table }
func (r OuterRef[T, V]) outerRefName() string  { return r.name }

// outerRefCore is the value-erasure every OuterRef[T,V] instantiation
// satisfies, letting toRenderNode convert a marker held in a generic Node's
// `any` Value slot without a runtime reflection over the exact pair of type
// parameters.
type outerRefCore interface {
	outerRefTable() string
	outerRefName() string
}

// Outer builds a correlated reference to c -- a column of the query this
// predicate will be nested INSIDE of, rendered fully qualified (outer table
// + column) against that directly enclosing query at execution time. For
// example, every product with zero purchases, the classic anti-join:
//
//	From(products).Where(orm.NotExists[product](
//		From(purchases).Where(purchases.ProductID.EqOuter(orm.Outer(products.ID))),
//	))
//
// Outer is only valid in the Where of a query used as a subquery operand
// (Exists/NotExists/in-subquery/scalar comparison) whose directly enclosing
// statement FROMs c's own table. A marker used anywhere else -- the outermost
// query's own Where without an enclosing subquery, under a statement shape
// that cannot be an enclosing query (setop, aggregate, CTE body, mutation),
// or naming a table outside the directly enclosing statement's tables -- is
// a typed rendering-time error reported by render, never silently-wrong
// SQL. A marker whose table name is shadowed by the subquery's own FROM
// table is likewise rejected rather than silently binding inside.
func Outer[T any, V any](c Column[T, V]) OuterRef[T, V] {
	return OuterRef[T, V](c)
}

// OuterNullable builds a correlated reference to a NULLable column c of the
// enclosing query; see Outer. The inner column may be either nullable or
// not: a marker built from a NullableColumn compares like any other --
// SQL's three-valued logic decides what a NULL outer value means
// (`NULL = x` is never true, `NULL != x` is never true either).
func OuterNullable[T any, V any](c NullableColumn[T, V]) OuterRef[T, V] {
	return OuterRef[T, V](c)
}
