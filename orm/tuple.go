package orm

// Tuple is a multi-column row-value expression over entity T -- the LHS of a
// `(a, b) [NOT] IN (SELECT ...)` membership predicate or a
// `(a, b) OP (SELECT ...)` row-value comparison. Build one with NewTuple from
// codegen-derived columns erased via Col(), then apply In/NotIn or one of the
// comparison methods with the inner subquery:
//
//	orm.NewTuple(widgetID.Col(), widgetQty.Col()).In(
//		orm.From(widgetOrders).Columns(orderWidgetID.Col(), woAmount.Col()),
//	)
//
// All of the tuple's columns belong to entity T (enforced at compile time by
// the shared AnyColumn[T] type parameter), so the tuple only composes with a
// predicate over T. The inner subquery is snapshotted at construction time and
// renders through the SAME dialect as the enclosing statement, with the
// enclosing placeholder counter threaded through -- exactly like the
// InSub/EqScalar subqueries (see orm/query.go). The tuple arity must match
// the subquery's projected column count, and at least one column is required;
// both are typed rendering-time errors, never silently-wrong SQL.
type Tuple[T any] struct {
	cols []string
}

// NewTuple builds a row-value tuple over cols. Every col must be a
// codegen-derived AnyColumn (from Column.Col()/NullableColumn.Col()), never a
// raw string; cols may mix value types because AnyColumn erases V, but they
// must all belong to entity T. At least one column is required.
func NewTuple[T any](cols ...AnyColumn[T]) Tuple[T] {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name()
	}

	return Tuple[T]{cols: names}
}

// In builds a `(a, b) IN (SELECT ...)` membership predicate. inner may be a
// query over any entity C; it must project exactly as many columns as the
// tuple has (via Query.Columns) -- a mismatch is a typed rendering-time error.
func (t Tuple[T]) In[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(In, inner)
}

// NotIn builds a `(a, b) NOT IN (SELECT ...)` membership predicate, carrying
// the negation INTO the clause (unlike Not(t.In(...)), which renders an outer
// `NOT ((...) IN (...))` wrapper). See In for the projection contract.
func (t Tuple[T]) NotIn[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(NotIn, inner)
}

// Eq builds a `(a, b) = (SELECT ...)` row-value comparison. inner must project
// exactly as many columns as the tuple has and, in SQL, must return exactly
// one row (a scalar row); a projection mismatch is a typed rendering-time
// error. Row-value comparison is valid on Postgres and SQLite (>= 3.15).
func (t Tuple[T]) Eq[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Eq, inner)
}

// Neq builds a `(a, b) != (SELECT ...)` row-value comparison; see Eq.
func (t Tuple[T]) Neq[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Neq, inner)
}

// Gt builds a `(a, b) > (SELECT ...)` row-value comparison; see Eq.
func (t Tuple[T]) Gt[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Gt, inner)
}

// Gte builds a `(a, b) >= (SELECT ...)` row-value comparison; see Eq.
func (t Tuple[T]) Gte[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Gte, inner)
}

// Lt builds a `(a, b) < (SELECT ...)` row-value comparison; see Eq.
func (t Tuple[T]) Lt[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Lt, inner)
}

// Lte builds a `(a, b) <= (SELECT ...)` row-value comparison; see Eq.
func (t Tuple[T]) Lte[C any, PC ptrScanner[C]](inner Query[C, PC]) Predicate[T] {
	return t.predicate(Lte, inner)
}

// predicate erases the tuple's column list and the inner query into an NTuple
// node. The inner query is snapshotted via toSubquery so a later branch on it
// cannot alter an already-built predicate (Query is an immutable value type).
func (t Tuple[T]) predicate[C any, PC ptrScanner[C]](op Op, inner Query[C, PC]) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NTuple, Op: op, Tuple: t.cols, Value: toSubquery(inner)}}
}
