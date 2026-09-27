package ormdrill

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoQueryLogger installs orm.SetQueryLogger around one query and prints the
// rendered SQL plus bound args. The hook runs synchronously on the executing
// goroutine right before the statement is issued; the restore func reinstates
// whatever logger was active before (here, none).
func DemoQueryLogger(ctx context.Context, conn db.DB) error {
	fmt.Println("== SetQueryLogger: capture one query")

	var captured []string

	restore := orm.SetQueryLogger(func(query string, args []any) {
		captured = append(captured, query+" "+fmt.Sprint(args))
	})
	defer restore()

	n, err := orm.From(Orders).Where(OrderCols.AmountCents.Gt(1000)).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("captured count: %w", err)
	}

	for _, q := range captured {
		fmt.Printf("  captured: %s\n", q)
	}

	fmt.Printf("  count (amount_cents > 1000) = %d\n\n", n)

	return nil
}
