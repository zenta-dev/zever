// Command app runs every ormdrill topic in sequence against one in-memory
// SQLite database, mirroring the per-topic Example tests. The order is
// load-bearing: price-mutating flows (returning, joined update) and the
// retry insert run after the flows that assert on pristine prices and
// shipment counts.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/examples/ormdrill"
)

func main() {
	ctx := context.Background()

	conn, err := ormdrill.OpenShop(ctx)
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	demos := []func(context.Context, db.DB) error{
		ormdrill.DemoQueryLogger,
		ormdrill.DemoCursorPagination,
		ormdrill.DemoPreload,
		ormdrill.DemoSubqueryPredicates,
		ormdrill.DemoCorrelatedOuterInJoin,
		ormdrill.DemoInsertSelectDistinct,
		ormdrill.DemoLockingGates,
		ormdrill.DemoScalarExpr,
		ormdrill.DemoProjection,
		ormdrill.DemoDistinctOnLocksTablesample,
		ormdrill.DemoMutationReturning,
		ormdrill.DemoTupleIn,
		ormdrill.DemoJoin3OuterMixed,
		ormdrill.DemoNullsOrdering,
		ormdrill.DemoUpsertWhere,
		ormdrill.DemoMutationOrderGate,
		ormdrill.DemoRetryTx,
		ormdrill.DemoFirstOrErr,
		ormdrill.DemoJoinOn3,
		ormdrill.DemoRightJoin,
		ormdrill.DemoJoinedUpdate,
	}

	for _, demo := range demos {
		if err := demo(ctx, conn); err != nil {
			die(err)
		}
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
