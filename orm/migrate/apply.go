package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

// This file implements statement execution and checksum bookkeeping, moved
// from orm/migrate/apply.go and orm/migrate/diff.go: Apply executes a
// previously-computed MigrationPlan, skipping any statement already recorded
// as applied and recording every newly-applied one.

// migrationTrackingColumns are the bookkeeping columns added to
// schema_migrations by the rollback feature, on top of the original
// (id, checksum, applied_at) shape that installs predating it already have.
//
// They are all nullable TEXT, deliberately: adding them to an existing table
// must not fail on rows already there, and a NULL here means "recorded before
// this feature shipped" -- which rollback reports and skips, never errors on.
var migrationTrackingColumns = []struct{ Name, Type string }{
	{"kind", "TEXT"},
	{"table_name", "TEXT"},
	{"column_name", "TEXT"},
	{"statement", "TEXT"},
	{"prior_type", "TEXT"},
	{"prior_name", "TEXT"},
	{"object_name", "TEXT"},
	{"prior_sql", "TEXT"},
}

// EnsureMigrationsTable creates the schema_migrations bookkeeping table if it
// does not already exist, then brings an older table's shape up to date by
// adding any missing tracking column.
//
// The second half is this feature eating its own cooking: schema_migrations
// is migrated by the very same introspect-then-conditionally-ADD-COLUMN
// pattern the tool applies to user tables. It has to be, because SQLite has no
// "ADD COLUMN IF NOT EXISTS" -- the only way to know whether a column is
// already there is to look.
//
// Apply calls this itself before executing a plan; a caller that needs the
// bookkeeping table ready before doing anything else (e.g. `zever db
// rollback` reading rows before Apply ever runs) may call it directly too --
// it is idempotent.
func EnsureMigrationsTable(ctx context.Context, conn db.DB, dialect string) error {
	var stmt string

	switch dialect {
	case atlas.DialectPostgres:
		stmt = `CREATE TABLE IF NOT EXISTS schema_migrations (
  id BIGSERIAL PRIMARY KEY,
  checksum TEXT NOT NULL UNIQUE,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`
	case atlas.DialectSQLite:
		stmt = `CREATE TABLE IF NOT EXISTS schema_migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  checksum TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`
	case atlas.DialectMySQL:
		// checksum is a sha256 hex (64 chars), so it is VARCHAR(64) here:
		// MySQL rejects an index -- even the UNIQUE backing the dedup -- on
		// a TEXT/BLOB column without an explicit key length.
		stmt = `CREATE TABLE IF NOT EXISTS schema_migrations (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  checksum VARCHAR(64) NOT NULL UNIQUE,
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`
	default:
		return fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
	}

	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("[orm/migrate] create %s: %w", schemaMigrationsTable, err)
	}

	return addMigrationTrackingColumns(ctx, conn, dialect)
}

// addMigrationTrackingColumns adds every tracking column schema_migrations is
// missing, leaving existing rows (and their NULLs in the new columns) alone.
//
// Postgres can express this declaratively with ADD COLUMN IF NOT EXISTS.
// SQLite and MySQL cannot, so their live columns are introspected first
// through the same introspect*Columns the user-table diff uses.
func addMigrationTrackingColumns(ctx context.Context, conn db.DB, dialect string) error {
	present := map[string]bool{}

	switch dialect {
	case atlas.DialectSQLite:
		cols, err := introspectSQLiteColumns(ctx, conn, schemaMigrationsTable)
		if err != nil {
			return err
		}

		for _, c := range cols {
			present[c.Name] = true
		}
	case atlas.DialectMySQL:
		cols, err := introspectMySQLColumns(ctx, conn, schemaMigrationsTable)
		if err != nil {
			return err
		}

		for _, c := range cols {
			present[c.Name] = true
		}
	}

	for _, col := range migrationTrackingColumns {
		if present[col.Name] {
			continue
		}

		var stmt string

		switch dialect {
		case atlas.DialectPostgres:
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;",
				schemaMigrationsTable, col.Name, col.Type)
		case atlas.DialectSQLite:
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;",
				schemaMigrationsTable, col.Name, col.Type)
		case atlas.DialectMySQL:
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;",
				schemaMigrationsTable, quoteIdent(dialect, col.Name), col.Type)
		default:
			return fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
		}

		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("[orm/migrate] add %s.%s: %w", schemaMigrationsTable, col.Name, err)
		}
	}

	return nil
}

// ChecksumOf returns the hex-encoded sha256 of a DDL statement's exact text,
// used as the schema_migrations dedup key (content-based idempotency, since
// this tool has no migration files to number).
func ChecksumOf(stmt string) string {
	sum := sha256.Sum256([]byte(stmt))
	return hex.EncodeToString(sum[:])
}

// migrationApplied reports whether a statement with this checksum has
// already been recorded as applied.
func migrationApplied(ctx context.Context, conn db.DB, checksum string) (bool, error) {
	rows, err := conn.Query(ctx, "SELECT 1 FROM schema_migrations WHERE checksum = ? LIMIT 1", checksum)
	if err != nil {
		return false, fmt.Errorf("[orm/migrate] query %s: %w", schemaMigrationsTable, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	return rows.Next(), nil
}

// recordMigration records a successfully applied statement's checksum along
// with the metadata a rollback needs to invert it.
func recordMigration(ctx context.Context, conn db.DB, checksum string, meta migrationMeta) error {
	const q = `INSERT INTO schema_migrations
  (checksum, kind, table_name, column_name, statement, prior_type, prior_name, object_name, prior_sql)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := conn.Exec(ctx, q,
		checksum, meta.Kind, meta.Table, meta.Column, meta.Statement, meta.PriorType, meta.PriorName,
		meta.ObjectName, meta.PriorSQL,
	); err != nil {
		return fmt.Errorf("[orm/migrate] record %s: %w", schemaMigrationsTable, err)
	}

	return nil
}

// Apply executes a plan computed by Plan: it ensures the schema_migrations
// bookkeeping table exists (see EnsureMigrationsTable), then executes every
// statement whose checksum is not already recorded, recording each as it
// applies. Returns the count of statements actually executed -- a re-run of
// an already-applied plan returns 0 with a nil error.
func Apply(ctx context.Context, exec db.DB, plan *MigrationPlan) (int, error) {
	if plan == nil {
		return 0, errors.New("[orm/migrate] Apply called with a nil plan")
	}

	if err := EnsureMigrationsTable(ctx, exec, plan.Dialect); err != nil {
		return 0, err
	}

	return applyMigrationPlan(ctx, exec, plan.statements)
}

// supportsTransactionalDDL reports whether dialect can run a DDL statement
// and a subsequent bookkeeping INSERT atomically inside one transaction.
//
// Postgres and SQLite both support transactional DDL: a ROLLBACK undoes the
// schema change along with any other writes in the transaction. MySQL/InnoDB
// does not -- DDL statements there cause an implicit commit of the current
// transaction (and cannot themselves be rolled back), so wrapping them in a
// transaction would provide no atomicity and would be misleading to a
// reader of this code.
func supportsTransactionalDDL(dialect string) bool {
	switch dialect {
	case atlas.DialectPostgres, atlas.DialectSQLite:
		return true
	case atlas.DialectMySQL:
		return false
	default:
		return false
	}
}

// applyMigrationPlan executes each statement in order, skipping any whose
// checksum is already recorded in schema_migrations, and recording every
// newly-applied statement's checksum. Returns the count actually executed.
//
// On a dialect with transactional DDL (Postgres, SQLite), each statement's
// exec and its recordMigration bookkeeping insert are wrapped in one
// transaction (one transaction per statement, not one spanning the whole
// plan -- that would hold locks for the plan's entire duration and buys
// nothing extra). This makes the pair atomic: if the process crashes or the
// connection drops between the exec succeeding and the bookkeeping insert
// completing, the transaction is never committed, so the DDL's effect is
// rolled back too and the next Apply call correctly re-attempts the whole
// statement -- it never sees a statement recorded as applied without having
// actually run, nor a statement that ran but was never recorded (which
// would cause a non-idempotent statement like DROP COLUMN to be re-executed
// and fail, or silently corrupt state).
//
// MySQL statements (and any exec that does not implement db.Transactor --
// e.g. a minimal test double) keep the previous unwrapped behavior: exec
// then record as two separate calls, with no transaction around either.
func applyMigrationPlan(ctx context.Context, conn db.DB, stmts []plannedStatement) (int, error) {
	applied := 0

	_, canTx := conn.(db.Transactor)
	useTx := canTx && supportsTransactionalDDL(dialectOf(conn))

	for _, stmt := range stmts {
		sum := ChecksumOf(stmt.SQL)

		done, err := migrationApplied(ctx, conn, sum)
		if err != nil {
			return applied, err
		}

		if done {
			continue
		}

		if useTx {
			if err := applyStatementInTx(ctx, conn, stmt, sum); err != nil {
				return applied, err
			}
		} else {
			if err := applyStatementUnguarded(ctx, conn, stmt, sum); err != nil {
				return applied, err
			}
		}

		applied++
	}

	return applied, nil
}

// dialectOf reports conn's dialect, tolerating a minimal test double that
// does not implement a Dialect() method by treating it as unrecognized
// (which supportsTransactionalDDL then reports as false).
func dialectOf(conn db.DB) string {
	if conn == nil {
		return ""
	}

	return conn.Dialect()
}

// applyStatementInTx executes one statement and its bookkeeping insert
// atomically: db.WithTx commits only if both succeed, and rolls back
// (undoing the DDL too, on a dialect where that is transactional) if either
// fails.
func applyStatementInTx(ctx context.Context, conn db.DB, stmt plannedStatement, sum string) error {
	err := db.WithTx(ctx, conn, nil, func(txCtx context.Context, tx db.Tx) error {
		if _, err := tx.Exec(txCtx, stmt.SQL); err != nil {
			return fmt.Errorf("[orm/migrate] exec %q: %w", firstLine(stmt.SQL), err)
		}

		if err := recordMigration(txCtx, tx, sum, stmt.Meta); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// applyStatementUnguarded is the pre-existing, non-transactional exec +
// record pair, used for dialects without transactional DDL (MySQL) and for
// any executor that does not implement db.Transactor.
func applyStatementUnguarded(ctx context.Context, conn db.DB, stmt plannedStatement, sum string) error {
	if _, err := conn.Exec(ctx, stmt.SQL); err != nil {
		return fmt.Errorf("[orm/migrate] exec %q: %w", firstLine(stmt.SQL), err)
	}

	if err := recordMigration(ctx, conn, sum, stmt.Meta); err != nil {
		return err
	}

	return nil
}

// firstLine returns a statement's first line, for compact error messages.
func firstLine(stmt string) string {
	for i := 0; i < len(stmt); i++ {
		if stmt[i] == '\n' {
			return stmt[:i]
		}
	}

	return stmt
}
