package orm

import "github.com/zenta-dev/zever/orm/render"

// subquery is the erased, render-ready inner SELECT carried by a subquery
// predicate node -- an In/NotIn-subquery (an NIn node whose Value is a
// subquery), an EXISTS predicate (an NSubquery node), or a scalar
// comparison against a subquery (an NBinary node whose Value is a
// subquery) -- in its Node's Value field.
//
// It is built at predicate-construction time by toSubquery from the inner
// Query: table, projected columns, where and order are converted to render's
// erased shapes THEN, while the SQL TEXT is still only produced at execution
// time, against the dialect the ENCLOSING query resolves and with the
// enclosing statement's placeholder counter threaded through. A subquery
// predicate therefore always renders through the same dialect as its outer
// query -- never from pre-rendered text captured at construction time.
type subquery struct {
	table   string
	columns []string
	where   render.Node
	order   []render.OrderTerm
	limit   int
	offset  int
}

// toSubquery snapshots an inner Query's selectable shape into a subquery
// value. inner is passed by value -- Query is an immutable value type
// -- so the predicate carries a fixed snapshot, and later
// branching on the inner Query (Where/OrderBy/Limit/Offset) cannot alter an
// already-built predicate.
func toSubquery[C any, PC ptrScanner[C]](inner Query[C, PC]) subquery {
	return subquery{
		table:   inner.table.Name(),
		columns: inner.selectColumns(),
		where:   toRenderNode[C](inner.where.Render()),
		order:   toRenderOrder(inner.order),
		limit:   inner.limit,
		offset:  inner.offset,
	}
}
