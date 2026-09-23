package ormdrill

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoJoin3OuterMixed exercises the homogeneous LeftJoinOn3 and the two mixed
// INNER/LEFT chains, then the RIGHT/FULL three-table builders. The Option per
// joined side is what makes an unmatched row unambiguous; the LEFT chain adds
// the invariant C.IsSome() implies B.IsSome(). RIGHT/FULL hops run where the
// SQLite library reports support and report the typed capability gate
// otherwise, so the demo stays green on older drivers too.
func DemoJoin3OuterMixed(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 6/7: three-table outer + mixed-kind joins")

	lefts, err := orm.LeftJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("LeftJoinOn3: %w", err)
	}

	bSome, cSome := 0, 0

	for _, r := range lefts {
		if r.B.IsSome() {
			bSome++
		}

		if r.C.IsSome() {
			cSome++
		}
	}

	fmt.Printf("  LeftJoinOn3:      %d rows, B present %d, C present %d\n", len(lefts), bSome, cSome)

	innerLeft, err := orm.InnerLeftJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("InnerLeftJoinOn3: %w", err)
	}

	cSome = 0

	for _, r := range innerLeft {
		if r.C.IsSome() {
			cSome++
		}
	}

	fmt.Printf("  InnerLeftJoinOn3: %d rows, C present %d\n", len(innerLeft), cSome)

	leftInner, err := orm.LeftInnerJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("LeftInnerJoinOn3: %w", err)
	}

	bSome = 0

	for _, r := range leftInner {
		if r.B.IsSome() {
			bSome++
		}
	}

	fmt.Printf("  LeftInnerJoinOn3: %d rows, B present %d\n", len(leftInner), bSome)

	// MixedJoinOn3 covers every (abKind, bcKind) pair uniformly with a
	// fully-optional Row3[Option, Option, Option] result. INNER/LEFT run on
	// every dialect; a RIGHT/FULL hop is the typed gate below on old ones.
	generic, err := orm.MixedJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments, orm.InnerJoin, orm.LeftJoin).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("MixedJoinOn3: %w", err)
	}

	cSome = 0

	for _, r := range generic {
		if r.C.IsSome() {
			cSome++
		}
	}

	fmt.Printf("  MixedJoinOn3:     %d rows (inner/left), C present %d\n", len(generic), cSome)

	mixedRight, err := orm.MixedJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments, orm.LeftJoin, orm.RightJoin).All(ctx, conn)
	if err != nil {
		fmt.Printf("  MixedJoinOn3 (left/right) -> rejected: %v\n", err)
	} else {
		fmt.Printf("  MixedJoinOn3:     %d rows (left/right)\n", len(mixedRight))
	}

	rights, err := orm.RightJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments).All(ctx, conn)
	if err != nil {
		fmt.Printf("  RightJoinOn3 -> rejected: %v\n", err)
	} else {
		fmt.Printf("  RightJoinOn3:     %d rows\n", len(rights))
	}

	fulls, err := orm.FullJoinOn3(orm.From(Widgets), WidgetOrders, OrderShipments).All(ctx, conn)
	if err != nil {
		fmt.Printf("  FullJoinOn3 -> rejected: %v\n", err)
	} else {
		fmt.Printf("  FullJoinOn3:      %d rows\n", len(fulls))
	}

	fmt.Println()

	return nil
}
