package ormdrill

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoJoinOn3 runs one INNER three-way join -- widgets -> orders ->
// shipments -- through orm.JoinOn3, filtering on the right tables with
// WhereRight/WhereC. This is a projection join (one SELECT, one rows.Scan
// per row into a typed Row3), the composite-row shape Preload deliberately
// does not use. Rows are ordered by the B side so the output is stable.
func DemoJoinOn3(ctx context.Context, conn db.DB) error {
	fmt.Println("== Join3: widgets -> orders -> shipments (inner)")

	rows, err := orm.JoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments, orm.InnerJoin).
		Where(orm.Contains(WidgetCols.Name, "gadget")).
		WhereRight(OrderCols.AmountCents.Gte(1000)).
		WhereC(ShipmentCols.Carrier.Eq("hermes")).
		OrderByRight(OrderCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("join3: %w", err)
	}

	for _, r := range rows {
		fmt.Printf("  %s -> %s (%d cents) -> %s\n", r.A.ID, r.B.ID, r.B.AmountCents, r.C.Tracking)
	}

	fmt.Printf("  %d matching rows (1 SELECT, 1 scan per row)\n\n", len(rows))

	return nil
}

// DemoRightJoin demonstrates a null-safe RightJoinOn. SQLite gained
// RIGHT/FULL OUTER JOIN in 3.39.0; when the bundled driver reports support
// the join runs, otherwise the typed dialect.ErrUnsupportedByDialect gate is
// reported instead of shipping SQL the engine cannot parse.
func DemoRightJoin(ctx context.Context, conn db.DB) error {
	fmt.Println("== RightJoin2 on SQLite: null-safe RIGHT JOIN")

	rows, err := orm.RightJoinOn(orm.From(Widgets), WidgetOrders).All(ctx, conn)
	if err != nil {
		fmt.Printf("  rejected: %v\n\n", err)

		return nil
	}

	orphans := 0

	for _, r := range rows {
		if !r.A.IsSome() {
			orphans++
		}
	}

	fmt.Printf("  %d rows, %d with no matching widget (A = None)\n\n", len(rows), orphans)

	return nil
}

// DemoJoinedUpdate uses the joined-mutation surface: UPDATE ... FROM rendered
// from Update.Join, scoped by the typed relation. The join is INNER (the
// FROM form can only express an inner join), so rows affected = the 5 widgets
// that have at least one order. Only the update-target's columns are
// reachable through Where (Update[T].Where takes a Predicate[T]); the
// joined-table scope is the join itself.
func DemoJoinedUpdate(ctx context.Context, conn db.DB) error {
	fmt.Println("== Update.Join: UPDATE widgets SET ... FROM orders (sqlite)")

	n, err := orm.UpdateTable(Widgets).
		Join(WidgetOrders, orm.InnerJoin).
		Set(orm.Set(WidgetCols.PriceCents, 1299)).
		Exec(ctx, conn)
	if err != nil {
		return fmt.Errorf("joined update: %w", err)
	}

	if n != 5 {
		return fmt.Errorf("joined update affected %d rows, want 5 (widgets with orders)", n)
	}

	count, err := orm.From(Widgets).Where(WidgetCols.PriceCents.Eq(1299)).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("count updated widgets: %w", err)
	}

	fmt.Printf("  %d widgets affected (the 5 with orders); now priced at 1299: count = %d\n\n", n, count)

	return nil
}
