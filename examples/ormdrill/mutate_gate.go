package ormdrill

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// reportGate prints a capability-gate result and fails if the error was not
// the expected typed rejection.
func reportGate(label string, err, want error) error {
	if err == nil {
		return fmt.Errorf("%s: expected %s, got nil", label, want.Error())
	}

	if !errors.Is(err, want) {
		return fmt.Errorf("%s: got %s, want errors.Is(_, %s)", label, err.Error(), want.Error())
	}

	fmt.Printf("  %-30s -> rejected: %v\n", label, err)

	return nil
}

// DemoInsertSelectDistinct copies a filtered set of widgets into an archive
// table with Insert.Columns + Insert.Select, then inserts one exact duplicate
// so Query.Distinct has something to remove.
func DemoInsertSelectDistinct(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 5: INSERT ... SELECT + Query.Distinct")

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets_archive (id text, name text, price_cents integer, created_at text, note text)`); err != nil {
		return fmt.Errorf("create widgets_archive: %w", err)
	}

	if err := orm.InsertInto(WidgetsArchive).
		Columns(
			WidgetCols.ID.Col(),
			WidgetCols.Name.Col(),
			WidgetCols.PriceCents.Col(),
			WidgetCols.CreatedAt.Col(),
			WidgetCols.Note.Col(),
		).
		Select(orm.From(Widgets).Where(WidgetCols.PriceCents.Gt(700))).
		Exec(ctx, conn); err != nil {
		return fmt.Errorf("insert select: %w", err)
	}

	// A deliberate exact duplicate so DISTINCT has something to remove.
	if _, err := conn.Exec(ctx, `INSERT INTO widgets_archive SELECT * FROM widgets_archive WHERE id = ?`, "w08"); err != nil {
		return fmt.Errorf("duplicate archive row: %w", err)
	}

	all, err := orm.From(WidgetsArchive).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("archive all: %w", err)
	}

	unique, err := orm.From(WidgetsArchive).Distinct().All(ctx, conn)
	if err != nil {
		return fmt.Errorf("archive distinct: %w", err)
	}

	fmt.Printf("  INSERT ... SELECT copied the %d widgets priced > 700\n", len(unique))
	fmt.Printf("  archive rows = %d, SELECT DISTINCT rows = %d\n\n", len(all), len(unique))

	return nil
}

// DemoLockingGates shows the typed gates that keep an invalid or unsupported
// lock from reaching SQLite: a lock the dialect lacks, DISTINCT combined with
// a lock, and a lock modifier (NOWAIT/SKIP LOCKED) with no lock mode.
func DemoLockingGates(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 5: row locking and DISTINCT (typed gates on SQLite)")

	_, err := orm.From(Widgets).ForUpdate().All(ctx, conn)
	if gateErr := reportGate("ForUpdate", err, dialect.ErrUnsupportedByDialect); gateErr != nil {
		return gateErr
	}

	_, err = orm.From(Widgets).Distinct().ForUpdate().All(ctx, conn)
	if gateErr := reportGate("Distinct + ForUpdate", err, orm.ErrLockingWithDistinct); gateErr != nil {
		return gateErr
	}

	_, err = orm.From(Widgets).NoWait().All(ctx, conn)
	if gateErr := reportGate("NoWait without a lock", err, orm.ErrLockingRequiresLockMode); gateErr != nil {
		return gateErr
	}

	_, err = orm.From(Widgets).ForShare().SkipLocked().All(ctx, conn)
	if gateErr := reportGate("ForShare + SkipLocked", err, dialect.ErrUnsupportedByDialect); gateErr != nil {
		return gateErr
	}

	fmt.Println()

	return nil
}

// DemoDistinctOnLocksTablesample shows the Postgres-only SELECT modifiers.
// SQLite rejects every one with the typed dialect.ErrUnsupportedByDialect
// rather than shipping SQL the engine cannot parse.
func DemoDistinctOnLocksTablesample(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 8: DISTINCT ON / extended locks / TABLESAMPLE (Postgres-only)")

	checks := []struct {
		label string
		run   func() error
	}{
		{"DISTINCT ON", func() error {
			_, err := orm.From(Widgets).
				DistinctOn(WidgetCols.Name.Col()).
				OrderBy(WidgetCols.Name.Asc(), WidgetCols.ID.Desc()).
				All(ctx, conn)

			return err
		}},
		{"FOR NO KEY UPDATE", func() error {
			_, err := orm.From(Widgets).ForNoKeyUpdate().All(ctx, conn)

			return err
		}},
		{"FOR KEY SHARE", func() error {
			_, err := orm.From(Widgets).ForKeyShare().All(ctx, conn)

			return err
		}},
		{"FOR UPDATE OF", func() error {
			_, err := orm.From(Widgets).ForUpdateOf(WidgetCols.ID.Col()).All(ctx, conn)

			return err
		}},
		{"TABLESAMPLE", func() error {
			_, err := orm.From(Widgets).Tablesample("SYSTEM", 10).All(ctx, conn)

			return err
		}},
	}

	for _, c := range checks {
		if err := reportGate(c.label, c.run(), dialect.ErrUnsupportedByDialect); err != nil {
			return err
		}
	}

	fmt.Println()

	return nil
}

// DemoMutationOrderGate shows that a single-table UPDATE/DELETE ORDER BY/LIMIT
// tail is a MySQL extension: on SQLite Exec rejects it with the typed
// dialect.ErrUnsupportedByDialect instead of shipping SQL the engine cannot
// parse.
func DemoMutationOrderGate(ctx context.Context, conn db.DB) error {
	fmt.Println("== Round 4: mutation ORDER BY / LIMIT is MySQL-only")

	_, err := orm.UpdateTable(Widgets).
		Set(orm.Set(WidgetCols.PriceCents, int64(1))).
		OrderBy(WidgetCols.ID.Asc()).
		Limit(1).
		Exec(ctx, conn)
	if gateErr := reportGate("UPDATE ... ORDER BY/LIMIT", err, dialect.ErrUnsupportedByDialect); gateErr != nil {
		return gateErr
	}

	_, err = orm.DeleteFrom(Widgets).
		OrderBy(WidgetCols.ID.Asc()).
		Limit(1).
		Exec(ctx, conn)
	if gateErr := reportGate("DELETE ... ORDER BY/LIMIT", err, dialect.ErrUnsupportedByDialect); gateErr != nil {
		return gateErr
	}

	fmt.Println()

	return nil
}
