package ormdrill

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoScalarExpr roots COALESCE/NULLIF/LOWER/UPPER/TRIM/LENGTH/CASE
// expressions into WHERE predicates and counts the matches. The expression
// value type is compile-time pinned, so each comparison only compiles against
// a same-typed value.
func DemoScalarExpr(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 5: typed scalar expressions (COALESCE/LOWER/UPPER/TRIM/LENGTH/NULLIF/CASE)")

	countWhere := func(label string, p orm.Predicate[Widget]) error {
		n, err := orm.From(Widgets).Where(p).Count(ctx, conn)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}

		fmt.Printf("  %-38s = %d\n", label, n)

		return nil
	}

	if err := countWhere("LOWER(name) = gadget-01", orm.Lower(WidgetCols.Name.Expr()).Eq("gadget-01")); err != nil {
		return err
	}

	if err := countWhere("UPPER(name) = GADGET-02", orm.Upper(WidgetCols.Name.Expr()).Eq("GADGET-02")); err != nil {
		return err
	}

	if err := countWhere("TRIM(name) = gadget-03", orm.Trim(WidgetCols.Name.Expr()).Eq("gadget-03")); err != nil {
		return err
	}

	if err := countWhere("LENGTH(name) = 9", orm.Length(WidgetCols.Name.Expr()).Eq(9)); err != nil {
		return err
	}

	if err := countWhere("NULLIF(name,'gadget-01') IS NULL", orm.NullIf(WidgetCols.Name.Expr(), "gadget-01").IsNull()); err != nil {
		return err
	}

	if err := countWhere("COALESCE(note,'n/a') = n/a", orm.CoalesceNullable(WidgetCols.Note, "n/a").Eq("n/a")); err != nil {
		return err
	}

	premium := orm.Case[Widget, string]().
		When(WidgetCols.PriceCents.Gte(800), "premium").
		Else("standard")

	if err := countWhere("CASE price >= 800 -> premium", premium.Eq("premium")); err != nil {
		return err
	}

	fmt.Println()

	return nil
}

// DemoProjection projects aliased scalar expressions instead of entity
// columns and scans each row positionally through ProjectedRow, then projects
// a correlated scalar subquery as an aliased output column.
func DemoProjection(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 8: expression projection + aliases")

	type widgetView struct{ ID, Note, Name, Tier string }

	var views []widgetView

	err := orm.From(Widgets).
		OrderBy(WidgetCols.ID.Asc()).
		Project(
			WidgetCols.ID.As("id"),
			orm.CoalesceNullable(WidgetCols.Note, "(none)").As("note"),
			orm.Lower(WidgetCols.Name.Expr()).As("name"),
			orm.Case[Widget, string]().
				When(WidgetCols.PriceCents.Gte(800), "premium").
				Else("standard").
				As("tier"),
		).
		All(ctx, conn, func(row orm.ProjectedRow) error {
			var v widgetView
			if err := row.Scan(&v.ID, &v.Note, &v.Name, &v.Tier); err != nil {
				return err
			}

			views = append(views, v)

			return nil
		})
	if err != nil {
		return fmt.Errorf("projection All: %w", err)
	}

	fmt.Printf("  projected %d widgets", len(views))

	if len(views) > 0 {
		fmt.Printf("; first: id=%s name=%s note=%s tier=%s", views[0].ID, views[0].Name, views[0].Note, views[0].Tier)
	}

	fmt.Println()

	// A correlated scalar subquery projects as an aliased output column: each
	// widget's largest order amount (NULL when it has no orders).
	largest := orm.From(Orders).
		Columns(OrderCols.AmountCents.Col()).
		Where(OrderCols.WidgetID.EqOuter(orm.Outer(WidgetCols.ID))).
		OrderBy(OrderCols.AmountCents.Desc()).
		Limit(1)

	var (
		firstID     string
		firstAmount orm.Option[int64]
	)

	ok, err := orm.From(Widgets).
		OrderBy(WidgetCols.ID.Asc()).
		Project(
			WidgetCols.ID.As("id"),
			orm.ScalarSubquery[Widget, int64](largest).As("largest_amount"),
		).
		First(ctx, conn, &firstID, &firstAmount)
	if err != nil {
		return fmt.Errorf("projection First: %w", err)
	}

	amount := "NULL"
	if v, some := firstAmount.Get(); some {
		amount = strconv.FormatInt(v, 10)
	}

	fmt.Printf("  First(ok=%v): widget %s largest order amount = %s\n\n", ok, firstID, amount)

	return nil
}

// DemoNullsOrdering orders widgets by their nullable note with NULLs forced
// to each end. SQLite supports the modifier natively; dialects without the
// syntax report a typed error instead.
func DemoNullsOrdering(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 6: ORDER BY ... NULLS FIRST/LAST (native on SQLite)")

	first, err := orm.From(Widgets).
		OrderBy(WidgetCols.Note.Asc().NullsFirst(), WidgetCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("nulls first: %w", err)
	}

	fmt.Print("  NULLS FIRST:")

	for _, w := range first {
		fmt.Printf(" %s:%s", w.ID, w.Note.GetOr("NULL"))
	}

	fmt.Println()

	last, err := orm.From(Widgets).
		OrderBy(WidgetCols.Note.Asc().NullsLast(), WidgetCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("nulls last: %w", err)
	}

	fmt.Print("  NULLS LAST: ")

	for _, w := range last {
		fmt.Printf(" %s:%s", w.ID, w.Note.GetOr("NULL"))
	}

	fmt.Println()
	fmt.Println()

	return nil
}
