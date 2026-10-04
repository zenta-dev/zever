package ormdrill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// ErrMutationReturning indicates the RETURNING mutation demo failed.
var ErrMutationReturning = errors.New("mutation returning")

// ErrUpsertWhere indicates the partial upsert WHERE demo failed.
var ErrUpsertWhere = errors.New("upsert where")

// DemoMutationReturning extends the INSERT-only RETURNING story to mutations,
// scanning the post-write rows back through Widget.Scan. Returning() with no
// arguments projects every column so the positional scan lines up.
func DemoMutationReturning(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 3: UPDATE / DELETE ... RETURNING (Postgres/SQLite)")

	updated, err := orm.UpdateTable(Widgets).
		Where(WidgetCols.ID.Eq("w08")).
		Set(orm.Set(WidgetCols.PriceCents, int64(9999))).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return fmt.Errorf("ormdrill: %w update: %w", ErrMutationReturning, err)
	}

	if len(updated) != 1 {
		return fmt.Errorf("ormdrill: %w update: got %d rows, want 1", ErrMutationReturning, len(updated))
	}

	fmt.Printf("  UPDATE w08 -> %s now %d cents (post-write values)\n", updated[0].ID, updated[0].PriceCents)

	if insertErr := orm.InsertInto(Widgets).Values(
		orm.Set(WidgetCols.ID, "w99"),
		orm.Set(WidgetCols.Name, "gadget-99"),
		orm.Set(WidgetCols.PriceCents, int64(42)),
		orm.Set(WidgetCols.CreatedAt, time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)),
	).Exec(ctx, conn); insertErr != nil {
		return fmt.Errorf("ormdrill: insert throwaway widget: %w", insertErr)
	}

	deleted, err := orm.DeleteFrom(Widgets).
		Where(WidgetCols.ID.Eq("w99")).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return fmt.Errorf("ormdrill: %w delete: %w", ErrMutationReturning, err)
	}

	if len(deleted) != 1 {
		return fmt.Errorf("ormdrill: %w delete: got %d rows, want 1", ErrMutationReturning, len(deleted))
	}

	fmt.Printf("  DELETE w99 ... RETURNING -> %s (%s)\n\n", deleted[0].ID, deleted[0].Name)

	return nil
}

// promoTargetWhere returns the conflict-target predicate naming the partial
// index promotions_code_active (WHERE active = 1). SQLite requires the target
// predicate to match the index definition literally at prepare time -- a
// bound parameter cannot be proven equivalent -- so the literal is expressed
// through the audited orm.UnsafeRaw escape hatch.
func promoTargetWhere() orm.Predicate[Promotion] {
	return orm.UnsafeRaw[Promotion]("active = 1")
}

// DemoUpsertWhere shows both optional upsert predicates: the partial-index
// target predicate on ON CONFLICT, and the conditional DO UPDATE WHERE, which
// applies the assignment only while the predicate holds over the existing
// row. Both are supported on Postgres and SQLite and typed errors on MySQL.
func DemoUpsertWhere(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 6: partial upsert WHERE (target predicate + DO UPDATE WHERE)")

	for _, stmt := range []string{
		`CREATE TABLE promotions (code text, discount_pct integer, active integer)`,
		`CREATE UNIQUE INDEX promotions_code_active ON promotions(code) WHERE active = 1`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ormdrill: %w DDL %q: %w", ErrUpsertWhere, stmt, err)
		}
	}

	if _, err := conn.Exec(ctx, `INSERT INTO promotions (code, discount_pct, active) VALUES (?, ?, ?)`, "SAVE10", int64(10), int64(1)); err != nil {
		return fmt.Errorf("ormdrill: %w: seed promotion: %w", ErrUpsertWhere, err)
	}

	readDiscount := func() (int64, error) {
		p, err := orm.From(Promotions).Where(PromotionCols.Code.Eq("SAVE10")).FirstOrErr(ctx, conn)
		if err != nil {
			return 0, err
		}

		return p.DiscountPct, nil
	}

	upsert := func(updatePred orm.Predicate[Promotion]) error {
		return orm.InsertInto(Promotions).
			Values(
				orm.Set(PromotionCols.Code, "SAVE10"),
				orm.Set(PromotionCols.DiscountPct, int64(99)),
				orm.Set(PromotionCols.Active, int64(1)),
			).
			OnConflict(PromotionCols.Code.Col()).
			Where(promoTargetWhere()).
			DoUpdate(orm.Set(PromotionCols.DiscountPct, int64(99))).
			Where(updatePred).
			Exec(ctx, conn)
	}

	// Existing discount is 10, so the update predicate (10 > 1000) is false
	// and the conflicting row keeps its value.
	if upsertErr := upsert(PromotionCols.DiscountPct.Gt(1000)); upsertErr != nil {
		return fmt.Errorf("ormdrill: %w (false predicate): %w", ErrUpsertWhere, upsertErr)
	}

	unchanged, err := readDiscount()
	if err != nil {
		return fmt.Errorf("ormdrill: %w: read after false predicate: %w", ErrUpsertWhere, err)
	}

	// Existing discount is still 10, so (10 < 1000) is true: the row updates.
	if upsertErr := upsert(PromotionCols.DiscountPct.Lt(1000)); upsertErr != nil {
		return fmt.Errorf("ormdrill: %w (true predicate): %w", ErrUpsertWhere, upsertErr)
	}

	updated, err := readDiscount()
	if err != nil {
		return fmt.Errorf("ormdrill: %w: read after true predicate: %w", ErrUpsertWhere, err)
	}

	fmt.Printf("  existing discount 10: predicate >1000 false -> %d; predicate <1000 true -> %d\n\n", unchanged, updated)

	return nil
}
