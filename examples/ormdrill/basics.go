package ormdrill

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoPreload fetches every widget and attaches its orders via orm.Preload in
// exactly two queries regardless of the row count. The query logger installed
// around the call proves the "2 queries" claim.
func DemoPreload(ctx context.Context, conn db.DB) error {
	fmt.Println("== Preload: widgets with their orders (N+1-free, 2 queries)")

	queries := 0

	restore := orm.SetQueryLogger(func(_ string, _ []any) {
		queries++
	})

	out, err := orm.Preload(ctx, conn,
		orm.From(Widgets).OrderBy(WidgetCols.ID.Asc()),
		OrderCols.WidgetID,
		orm.From(Orders).OrderBy(OrderCols.CreatedAt.Asc()),
		func(w *Widget) string { return w.ID },
		func(o *Order) string { return o.WidgetID },
	)
	restore()

	if err != nil {
		return fmt.Errorf("preload: %w", err)
	}

	if queries != 2 {
		return fmt.Errorf("preload ran %d queries, want exactly 2", queries)
	}

	fmt.Printf("  %d queries (parents all + one FK-IN fetch)\n", queries)

	for _, pc := range out {
		fmt.Printf("  %s (%s) has %d order(s):", pc.Parent.ID, pc.Parent.Name, len(pc.Children))

		for _, o := range pc.Children {
			fmt.Printf(" %s", o.ID)
		}

		fmt.Println()
	}

	fmt.Println()

	return nil
}

// DemoRetryTx runs one fully successful write through orm.RetryTx. A
// non-retryable failure would abort after a single attempt; a Postgres
// 40001/40P01 (or any error wrapping orm.ErrRetryable) would re-run the WHOLE
// transaction from a fresh BeginTx. Here the insert simply commits.
func DemoRetryTx(ctx context.Context, conn db.DB) error {
	fmt.Println("== RetryTx: a write that commits on the first attempt")

	err := orm.RetryTx(ctx, conn, orm.RetryOptions{}, func(ctx context.Context, tx db.Tx) error {
		insert := orm.InsertInto(Shipments).Values(
			orm.Set(ShipmentCols.ID, "s99"),
			orm.Set(ShipmentCols.OrderID, "o01"),
			orm.Set(ShipmentCols.Carrier, "fedex"),
			orm.Set(ShipmentCols.Tracking, "F999"),
		)

		return insert.Exec(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("retry tx: %w", err)
	}

	// The whole point of the exercise: the write survived the transaction.
	shipment, err := orm.From(Shipments).Where(ShipmentCols.ID.Eq("s99")).FirstOrErr(ctx, conn)
	if err != nil {
		return fmt.Errorf("read back retried shipment: %w", err)
	}

	fmt.Printf("  committed shipment %s for order %s (%s %s)\n", shipment.ID, shipment.OrderID, shipment.Carrier, shipment.Tracking)
	fmt.Printf("  IsRetryable(ErrRetryable) = %v (the caller-side escape hatch)\n", orm.IsRetryable(orm.ErrRetryable))

	fmt.Println()

	return nil
}

// DemoFirstOrErr exercises the error-returning single-row fetch: a hit
// returns the row, a miss returns an error testable with
// errors.Is(db.ErrNotFound) -- the "fetch by id or fail" shape.
func DemoFirstOrErr(ctx context.Context, conn db.DB) error {
	fmt.Println("== FirstOrErr: fetch-by-id, hit and db.ErrNotFound miss")

	w, err := orm.From(Widgets).Where(WidgetCols.ID.Eq("w01")).FirstOrErr(ctx, conn)
	if err != nil {
		return fmt.Errorf("firstOrErr hit: %w", err)
	}

	fmt.Printf("  hit: %s (%s, %d cents)\n", w.ID, w.Name, w.PriceCents)

	_, err = orm.From(Widgets).Where(WidgetCols.ID.Eq("w99")).FirstOrErr(ctx, conn)
	if err == nil {
		return errors.New("expected FirstOrErr miss error, got nil")
	}

	fmt.Printf("  miss: %v\n", err)
	fmt.Printf("  errors.Is(err, db.ErrNotFound) = %v\n\n", errors.Is(err, db.ErrNotFound))

	return nil
}
