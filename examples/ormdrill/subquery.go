package ormdrill

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// ErrSubquery indicates the subquery predicate demo failed.
var ErrSubquery = errors.New("ormdrill: subquery")

// ErrTupleIn indicates the tuple IN demo failed.
var ErrTupleIn = errors.New("ormdrill: tuple In")

// DemoSubqueryPredicates exercises the column-subquery surface:
// InSub/NotInSub membership, EXISTS/NOT EXISTS (the former correlated
// through orm.Outer), and a scalar EqScalar comparison against a one-row
// subquery. Each predicate snapshots its inner Query at construction time
// and renders through the enclosing statement's dialect.
func DemoSubqueryPredicates(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 3: subquery predicates (InSub / NotInSub / Exists / EqScalar)")

	withOrders, err := orm.From(Widgets).Where(WidgetCols.ID.InSub(
		orm.From(Orders).Columns(OrderCols.WidgetID.Col()),
	)).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w InSub", ErrSubquery), err)
	}

	withoutOrders, err := orm.From(Widgets).Where(WidgetCols.ID.NotInSub(
		orm.From(Orders).Columns(OrderCols.WidgetID.Col()),
	)).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w NotInSub", ErrSubquery), err)
	}

	shipped, err := orm.From(Orders).Where(orm.Exists[Order](
		orm.From(Shipments).Where(ShipmentCols.OrderID.EqOuter(orm.Outer(OrderCols.ID))),
	)).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w exists subquery", ErrSubquery), err)
	}

	missing, err := orm.From(Widgets).Where(orm.NotExists[Widget](
		orm.From(Orders).Where(OrderCols.WidgetID.EqOuter(orm.Outer(WidgetCols.ID))),
	)).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w NotExists", ErrSubquery), err)
	}

	refAmount, err := orm.From(Orders).Where(OrderCols.AmountCents.EqScalar(
		orm.From(Orders).Where(OrderCols.ID.Eq("o01")).Columns(OrderCols.AmountCents.Col()),
	)).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w EqScalar", ErrSubquery), err)
	}

	fmt.Printf("  widgets with orders (InSub)        = %d\n", withOrders)
	fmt.Printf("  widgets without orders (NotInSub)  = %d\n", withoutOrders)
	fmt.Printf("  orders with a shipment (Exists)    = %d\n", shipped)
	fmt.Printf("  widgets with no orders (NotExists) = %d\n", missing)
	fmt.Printf("  orders at o01's amount (EqScalar)  = %d\n\n", refAmount)

	return nil
}

// DemoCorrelatedOuterInJoin shows a correlated subquery placed in a
// projecting Join2's A-side Where referencing the join's B side (orders):
// the renderer resolves the marker against the join's table set.
func DemoCorrelatedOuterInJoin(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 7: correlated outer ref inside a projecting join")

	rows, err := orm.JoinOn(orm.From(Widgets), WidgetOrders, orm.InnerJoin).
		Where(orm.Exists[Widget](
			orm.From(Shipments).Where(ShipmentCols.OrderID.EqOuter(orm.Outer(OrderCols.ID))),
		)).
		OrderBy(WidgetCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w correlated join", ErrSubquery), err)
	}

	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		seen[r.A.ID] = true
	}

	fmt.Printf("  %d joined rows across %d distinct widgets whose order shipped\n\n", len(rows), len(seen))

	return nil
}

// DemoTupleIn uses a row-value tuple membership predicate: orders whose
// (widget_id, amount_cents) pair matches the pair of a reference order.
func DemoTupleIn(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 6: multi-column tuple row-value predicate")

	live, err := orm.From(Orders).Where(
		orm.NewTuple(OrderCols.WidgetID.Col(), OrderCols.AmountCents.Col()).In(
			orm.From(Orders).
				Where(OrderCols.ID.Eq("o01")).
				Columns(OrderCols.WidgetID.Col(), OrderCols.AmountCents.Col()),
		),
	).Count(ctx, conn)
	if err != nil {
		return errors.Join(fmt.Errorf("ormdrill: %w", ErrTupleIn), err)
	}

	fmt.Printf("  (widget_id, amount_cents) matching o01's pair = %d\n\n", live)

	return nil
}
