package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/orm/migrate"
)

// This file implements `zever db rollback`: flag parsing, prompts, and
// output formatting only. The actual rollback computation (reading recorded
// migrations, synthesizing an inverse for each, and applying the result)
// lives in orm/migrate's ComputeRollback/ApplyRollback -- see that
// package's rollback.go for the full "why it works this way" / "what is and
// is not recoverable" explanation this file used to carry inline.

// rollbackHelp is prepended to the flag usage of `zever db rollback`.
const rollbackHelp = `zever db rollback [flags...]

Undoes the most recent migration statements recorded in schema_migrations by
synthesizing an inverse for each one. Requires no .zen files: everything it
needs was recorded when the forward statement was applied.

!!! ROLLING BACK CAN DESTROY DATA. READ THIS BEFORE USING IT. !!!

  * undoing an ADD COLUMN means DROPPING that column. Every value written to
    it since it was added is discarded.
  * undoing a DROP COLUMN re-creates the column EMPTY. Its original data was
    destroyed by the drop and is NOT stored anywhere -- it is UNRECOVERABLE.
    You get the column back; you do not get the data back.
  * undoing an ALTER COLUMN TYPE restores the previous type but not values
    the forward cast truncated or rounded. Narrowing changes are one-way.
  * undoing a RENAME COLUMN, a CREATE/DROP INDEX, or an ADD FOREIGN KEY is
    lossless: none of them touch row data, only structure.
  * undoing an ALTER COLUMN NOT NULL restores the previous nullability, but
    going back to NOT NULL FAILS if a NULL was written to the column while it
    was relaxed -- it does not silently drop or invent rows.
  * CREATE TABLE has NO inverse and is always skipped: dropping a whole table
    that may hold data is deliberately out of scope for this command.

Every lossy statement prints a warning naming the affected column BEFORE it
is executed. Use --dry-run first to see the plan without touching anything.

-n COUNTS ROWS, NOT MIGRATE INVOCATIONS. A single "zever db migrate" run can
apply several statements, and each is one row in schema_migrations. "-n 1"
therefore undoes the single most recently applied STATEMENT, which may be
only part of what your last migrate command did. To undo a migrate run that
applied three statements, pass "-n 3".

Rows are processed newest-first (by id, the order they were applied in).
A row with no synthesizable inverse -- CREATE TABLE, or a row recorded before
this feature shipped and so missing its metadata -- is reported and skipped,
and stays in schema_migrations; the remaining rows are still rolled back.
Each successfully reversed row is deleted from schema_migrations, so a later
"zever db migrate" will apply the forward statement again.

Flags:`

// RollbackConfig is the resolved configuration for one `zever db rollback` run.
type RollbackConfig struct {
	// Adapter names the database backend (sqlite, postgres, or mysql).
	Adapter string
	// DSN is the connection string: a file path for sqlite, a libpq DSN for
	// postgres.
	DSN string
	// Count is how many schema_migrations ROWS (individual statements, not
	// migrate invocations) to undo, newest first.
	Count int
	// DryRun prints the inverse statements without executing them.
	DryRun bool
}

// resolveRollbackConfig builds a RollbackConfig from parsed flag values
// without side effects.
func resolveRollbackConfig(adapter, dsn string, count int, dryRun bool) RollbackConfig {
	return RollbackConfig{Adapter: adapter, DSN: dsn, Count: count, DryRun: dryRun}
}

// runDBRollback is the `zever db rollback` entrypoint.
func runDBRollback(args []string) error {
	fs := flag.NewFlagSet("db rollback", flag.ContinueOnError)
	adapter := fs.String("adapter", "sqlite", "db adapter to roll back (sqlite, postgres, or mysql)")
	dsn := fs.String("dsn", "", "connection string: a file path for sqlite, a libpq DSN for postgres")
	count := fs.Int("n", 1,
		"number of schema_migrations ROWS (individual statements, not migrate invocations) to undo")
	dryRun := fs.Bool("dry-run", false, "print the inverse statements without executing them")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), rollbackHelp)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := resolveRollbackConfig(*adapter, *dsn, *count, *dryRun)

	if cfg.Count < 1 {
		return fmt.Errorf("zever db rollback: -n must be at least 1, got %d", cfg.Count)
	}

	dialect, err := rollbackDialect(cfg.Adapter)
	if err != nil {
		return err
	}

	conn, err := openRollbackDB(cfg.Adapter, cfg.DSN)
	if err != nil {
		return err
	}

	ctx := context.Background()

	defer func() {
		_ = conn.Close(ctx)
	}()

	// The table may predate the tracking columns; ensure them before reading
	// columns that might not exist yet.
	if ensureErr := migrate.EnsureMigrationsTable(ctx, conn, dialect); ensureErr != nil {
		return ensureErr
	}

	plan, err := migrate.ComputeRollback(ctx, conn, cfg.Count)
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		warnRollback("%s", w)
	}

	if cfg.DryRun {
		for _, stmt := range plan.Statements {
			_, _ = fmt.Fprintln(os.Stdout, stmt)
		}

		return nil
	}

	executed, err := migrate.ApplyRollback(ctx, conn, plan)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout,
		"rolled back %d statement(s) via the %s adapter\n", executed, cfg.Adapter)

	return nil
}

// openRollbackDB opens the named adapter for rollback. It shares
// openZeverDB's registration but reports errors under the rollback command
// name.
func openRollbackDB(adapterName, dsn string) (db.DB, error) {
	ensureZeverDBAdapters()

	adapter, err := db.ParseAdapter(adapterName)
	if err != nil {
		return nil, fmt.Errorf("zever db rollback: unsupported --adapter %q (want sqlite, postgres, or mysql): %w", adapterName, err)
	}

	opts, err := rollbackOptions(adapterName, dsn)
	if err != nil {
		return nil, err
	}

	conn, err := db.Open(adapter, opts)
	if err != nil {
		return nil, fmt.Errorf("zever db rollback: open %s: %w", adapterName, err)
	}

	return conn, nil
}

// rollbackDialect maps a db adapter name to the SQL dialect whose inverse
// spelling to emit. MySQL is supported: the forward path records the same
// metadata, and rollback renders MySQL's inverse spellings (DROP INDEX ...
// ON <table>, DROP FOREIGN KEY, MODIFY COLUMN).
func rollbackDialect(adapter string) (string, error) {
	switch adapter {
	case "sqlite":
		return atlas.DialectSQLite, nil
	case "postgres":
		return atlas.DialectPostgres, nil
	case "mysql":
		return atlas.DialectMySQL, nil
	default:
		return "", fmt.Errorf(
			"zever db rollback: unsupported --adapter %q (want sqlite, postgres, or mysql)", adapter)
	}
}

// rollbackOptions builds the db.Options an adapter expects from --dsn,
// mirroring migrateOptions.
func rollbackOptions(adapter, dsn string) (db.Options, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return db.Options{}, errors.New("zever db rollback: --dsn is required")
	}

	if adapter == "sqlite" {
		return db.Options{Path: dsn}, nil
	}

	if adapter == "mysql" {
		return db.Options{}, errors.New("zever db rollback: --adapter mysql has no registered zever db adapter yet (sqlite and postgres only)")
	}

	return db.Options{DSN: dsn}, nil
}
