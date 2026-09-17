package migrate

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

// This file implements undoing the most recent DDL statements Apply applied,
// by SYNTHESIZING an inverse for each from the metadata recorded in
// schema_migrations. Moved from cmd/zengo/migrate_rollback.go.
//
// WHY IT WORKS THIS WAY. This tool has no migration files, so there is no
// "down()" to run: every forward statement is derived fresh from the current
// schema on every Plan. A rollback therefore cannot replay anything -- it
// has to reconstruct the opposite of what was recorded. That is why
// schema_migrations stores the statement's kind, table, column, and (where an
// inverse needs it) the prior type or prior name, captured at the only moment
// they are still observable: just before the forward statement ran.
//
// WHAT IS AND IS NOT RECOVERABLE:
//
//   - rename_column  -> LOSSLESS. Renaming back preserves every value.
//   - add_column     -> LOSSY. The inverse is a DROP: anything written to the
//                       column since it was added is discarded.
//   - drop_column    -> DATA IS UNRECOVERABLE. The inverse re-creates an
//                       EMPTY column of the recorded prior type. The original
//                       values were destroyed by the DROP and are not stored
//                       anywhere; nothing here can bring them back.
//   - alter_type     -> LOSSY for narrowing changes. The type is restored;
//                       values truncated or rounded by the forward cast are
//                       not. This is a limitation of DDL-level rollback in
//                       general, not of this implementation specifically.
//   - create_table   -> NO INVERSE, on purpose. Dropping a whole table that
//                       may hold data is materially more dangerous than any
//                       column-level undo, so it is out of scope.
//   - create_index   -> LOSSLESS. The inverse is a DROP INDEX; an index
//                       carries no data of its own, only a derived structure.
//   - drop_index     -> LOSSLESS. The inverse replays the exact CREATE INDEX
//                       statement captured before the drop ran.
//   - add_foreign_key -> LOSSLESS. The inverse is DROP CONSTRAINT; the
//                       constraint enforces nothing once removed, but every
//                       row's data is untouched either way.
//   - alter_nullability -> LOSSLESS in the DROP NOT NULL -> SET NOT NULL
//                       direction PROVIDED no NULL was written to the column
//                       in between -- if one was, restoring NOT NULL FAILS
//                       outright rather than silently losing data. The
//                       SET NOT NULL -> DROP NOT NULL direction is always
//                       lossless.
//
// Rows recorded before this feature existed have NULL kind/statement. They
// cannot be reconstructed, so they are reported and skipped -- never an error
// that would block rolling back newer, fully-tracked rows behind them.

// migrationRow is one schema_migrations row as read back for rollback.
// Every metadata field is empty when the underlying column is NULL, which is
// exactly what pre-tracking rows look like.
type migrationRow struct {
	ID         int64
	Checksum   string
	Kind       string
	Table      string
	Column     string
	Statement  string
	PriorType  string
	PriorName  string
	ObjectName string
	PriorSQL   string
}

// typeSpellingPattern is the defense-in-depth allowlist applied to a type
// spelling read back out of schema_migrations before it is interpolated into
// an inverse statement. Types cannot be bound as parameters in DDL, and while
// the values here were written by this tool from its own introspection, the
// check costs nothing.
var typeSpellingPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ ,()]*$`)

// RollbackPlan is the result of ComputeRollback: the ordered statements that
// undo the n most recently applied migration rows, plus every non-fatal
// notice (a lossy-rollback data-loss warning, or a row this tool could not
// synthesize an inverse for) noticed while computing it.
type RollbackPlan struct {
	// Dialect is the exec.Dialect() value ComputeRollback computed this plan
	// against.
	Dialect string
	// Statements are the inverse DDL and bookkeeping DELETE statements, in
	// execution order. Each successfully-invertible row contributes exactly
	// two: the inverse DDL, then a DELETE removing that row from
	// schema_migrations (see rollbackPlan's doc comment for why).
	Statements []string
	// Warnings are, in order: a data-loss notice printed BEFORE a lossy
	// inverse (add_column, drop_column, alter_type, and a NOT-NULL-restoring
	// alter_nullability), and a skip notice for any row with no
	// synthesizable inverse.
	Warnings []string
}

// ComputeRollback reads the n most recently applied schema_migrations rows
// (newest first, by id) and returns the statements that undo them, without
// executing anything. The caller should ensure the schema_migrations
// bookkeeping table already carries every tracking column (EnsureMigrationsTable)
// before calling this, since the table may predate them.
//
// Ordering is by id, not applied_at: applied_at has second granularity on
// sqlite (datetime('now')), so every statement of a single Apply call shares
// one timestamp and could not be ordered among themselves. id is monotonic per
// insert on both dialects (AUTOINCREMENT / BIGSERIAL).
func ComputeRollback(ctx context.Context, exec db.DB, n int) (*RollbackPlan, error) {
	dialect := exec.Dialect()

	switch dialect {
	case atlas.DialectPostgres, atlas.DialectSQLite, atlas.DialectMySQL:
	default:
		return nil, fmt.Errorf("[zen/migrate] dialect %q: %w", dialect, ErrUnsupportedDialect)
	}

	rows, err := readRecentMigrations(ctx, exec, n)
	if err != nil {
		return nil, err
	}

	plan := &RollbackPlan{Dialect: dialect}

	for _, row := range rows {
		stmt, ok := inverseStatement(dialect, row)
		if !ok {
			plan.Warnings = append(plan.Warnings, skipReason(row)+"; leaving it recorded")
			continue
		}

		if msg := lossyRollbackWarning(row); msg != "" {
			plan.Warnings = append(plan.Warnings, msg)
		}

		plan.Statements = append(plan.Statements, stmt,
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			fmt.Sprintf("DELETE FROM %s WHERE id = %d;", schemaMigrationsTable, row.ID))
	}

	return plan, nil
}

// skipReason explains, in the user's terms, why a row could not be inverted.
func skipReason(row migrationRow) string {
	switch row.Kind {
	case "":
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf("migration %d predates rollback tracking (no recorded kind or statement), so it cannot be undone", row.ID)
	case kindCreateTable:
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf("migration %d created a table; rolling back a whole table is out of scope", row.ID)
	default:
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf("migration %d (kind %q) has no synthesizable inverse from what was recorded", row.ID, row.Kind)
	}
}

// lossyRollbackWarning returns the data-loss consequence of one inverse, to
// be surfaced BEFORE it runs, or "" when the inverse is lossless. Rollback's
// failure mode is silent, cheerful destruction, so every lossy case says so
// up front and names the column it affects.
func lossyRollbackWarning(row migrationRow) string {
	switch row.Kind {
	case kindAddColumn:
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf("undoing the ADD of %s.%s DROPS that column: any data written to it since is discarded",
			row.Table, row.Column)
	case kindDropColumn:
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf(
			"re-adding %s.%s as an EMPTY %s column: its original data was destroyed by the DROP and is UNRECOVERABLE",
			row.Table, row.Column, row.PriorType)
	case kindAlterType:
		//lint:allow-unsafesql message text, not executed SQL
		return fmt.Sprintf("restoring the type of %s.%s to %s: values the forward cast truncated or rounded are not restored",
			row.Table, row.Column, row.PriorType)
	case kindAlterNullability:
		if row.PriorType == "NOT NULL" {
			//lint:allow-unsafesql message text, not executed SQL
			return fmt.Sprintf(
				"restoring NOT NULL on %s.%s: this FAILS if any NULL was written to it since the column was relaxed",
				row.Table, row.Column)
		}

		return ""
	default:
		return ""
	}
}

// inverseConstraintStatement handles the index/foreign-key/nullability kinds
// inverseStatement delegates to, split out to keep that function's
// cyclomatic complexity down rather than because these four share much logic
// with each other.
func inverseConstraintStatement(dialect string, row migrationRow) (string, bool) {
	switch row.Kind {
	case kindCreateIndex:
		if row.ObjectName == "" {
			return "", false
		}

		// MySQL spells the drop "DROP INDEX <name> ON <table>", with the
		// table recovered from the recorded row since MySQL has no
		// schema-qualified index name.
		if dialect == atlas.DialectMySQL {
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("DROP INDEX %s ON %s;", row.ObjectName, row.Table), true
		}

		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("DROP INDEX IF EXISTS %s;", row.ObjectName), true

	case kindDropIndex:
		if row.PriorSQL == "" || !createIndexPattern.MatchString(row.PriorSQL) {
			return "", false
		}

		return row.PriorSQL, true

	case kindAddForeignKey:
		if row.ObjectName == "" {
			return "", false
		}

		// MySQL drops a foreign key with DROP FOREIGN KEY (DROP CONSTRAINT
		// arrived only in 8.0.19 and changes nothing about the row data);
		// postgres uses DROP CONSTRAINT IF EXISTS, sqlite can never have
		// recorded this kind.
		if dialect == atlas.DialectMySQL {
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;", row.Table, row.ObjectName), true
		}

		ifExists := ""
		if dialect == atlas.DialectPostgres {
			ifExists = "IF EXISTS "
		}

		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s%s;", row.Table, ifExists, row.ObjectName), true

	case kindAlterNullability:
		// MySQL's forward MODIFY COLUMN recorded the whole prior column
		// definition (type + nullability) in PriorType, so its inverse is a
		// MODIFY COLUMN replaying that definition -- nothing like the
		// postgres ALTER COLUMN SET/DROP NOT NULL, which branches on the
		// recorded "NULL"/"NOT NULL" state below.
		if dialect == atlas.DialectMySQL {
			if !typeSpellingPattern.MatchString(row.PriorType) {
				return "", false
			}

			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s;",
				row.Table, quoteIdent(dialect, row.Column), row.PriorType), true
		}

		col := quoteIdent(dialect, row.Column)

		switch row.PriorType {
		case "NULL":
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", row.Table, col), true
		case "NOT NULL":
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", row.Table, col), true
		default:
			return "", false
		}

	default:
		return "", false
	}
}

// inverseStatement returns the DDL that undoes one recorded migration row, or
// ("", false) when the kind has no safe inverse: create_table, an unrecognized
// or NULL kind (a row recorded before this feature shipped), or a row missing
// the prior-state metadata its inverse would need.
//
// The table name is used verbatim: it was stored already-qualified and
// already-quoted by whichever render* function produced the forward statement.
// ObjectName is used verbatim the same way (see migrationMeta's doc comment).
// Column names and type spellings are validated and quoted here.
func inverseStatement(dialect string, row migrationRow) (string, bool) {
	if row.Table == "" {
		return "", false
	}

	switch row.Kind {
	case kindCreateIndex, kindDropIndex, kindAddForeignKey, kindAlterNullability:
		return inverseConstraintStatement(dialect, row)
	}

	// Every remaining kind needs a Column, validated once here rather than
	// in each case below.
	if row.Column == "" {
		return "", false
	}

	if err := validateIdent(row.Column); err != nil {
		return "", false
	}

	switch row.Kind {
	case kindAddColumn:
		ifExists := ""
		if dialect == atlas.DialectPostgres {
			ifExists = "IF EXISTS "
		}

		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s%s;",
			row.Table, ifExists, quoteIdent(dialect, row.Column)), true

	case kindDropColumn:
		if !typeSpellingPattern.MatchString(row.PriorType) {
			return "", false
		}

		ifNotExists := ""
		if dialect == atlas.DialectPostgres {
			ifNotExists = "IF NOT EXISTS "
		}

		// Deliberately no NOT NULL and no default: the column comes back
		// empty, and sqlite rejects adding a NOT NULL column without one.
		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s%s %s;",
			row.Table, ifNotExists, quoteIdent(dialect, row.Column), row.PriorType), true

	case kindRenameColumn:
		if validateIdent(row.PriorName) != nil {
			return "", false
		}

		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
			row.Table, quoteIdent(dialect, row.Column), quoteIdent(dialect, row.PriorName)), true

	case kindAlterType:
		// SQLite has no ALTER COLUMN TYPE, so it never records this kind in
		// the first place; the guard is here so a hand-edited row cannot make
		// the tool emit SQL sqlite would reject. MySQL records the full prior
		// column definition (see renderModifyColumn) and replays it with
		// MODIFY COLUMN; postgres uses ALTER COLUMN TYPE ... USING.
		if !typeSpellingPattern.MatchString(row.PriorType) {
			return "", false
		}

		if dialect == atlas.DialectMySQL {
			//lint:allow-unsafesql identifier is schema-introspected, not user input
			return fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s;",
				row.Table, quoteIdent(dialect, row.Column), row.PriorType), true
		}

		if dialect != atlas.DialectPostgres {
			return "", false
		}

		col := quoteIdent(dialect, row.Column)

		//lint:allow-unsafesql identifier is schema-introspected, not user input
		return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
			row.Table, col, row.PriorType, col, row.PriorType), true

	default:
		// create_table, NULL kind, or anything unrecognized.
		return "", false
	}
}

// ApplyRollback executes each statement of a rollback plan in order,
// stopping at the first failure. Unlike Apply there is no checksum dedup: an
// inverse is executed because the row it came from is still there, and the
// plan deletes that row immediately after. Returns the count of statements
// actually executed.
func ApplyRollback(ctx context.Context, exec db.DB, plan *RollbackPlan) (int, error) {
	if plan == nil {
		return 0, errors.New("[zen/migrate] ApplyRollback called with a nil plan")
	}

	executed := 0

	for _, stmt := range plan.Statements {
		if _, err := exec.Exec(ctx, stmt); err != nil {
			return executed, fmt.Errorf("[zen/migrate] exec %q: %w", firstLine(stmt), err)
		}

		executed++
	}

	return executed, nil
}

// readRecentMigrations reads the n most recently applied migration rows,
// newest first. NULLs in the tracking columns (pre-tracking rows) come back
// as empty strings, which inverseStatement treats as "not reconstructable".
func readRecentMigrations(ctx context.Context, conn db.DB, n int) ([]migrationRow, error) {
	const q = `SELECT id, checksum, kind, table_name, column_name, statement, prior_type, prior_name, object_name, prior_sql
  FROM schema_migrations ORDER BY id DESC LIMIT ?`

	rows, err := conn.Query(ctx, q, n)
	if err != nil {
		return nil, fmt.Errorf("[zen/migrate] query %s: %w", schemaMigrationsTable, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var out []migrationRow

	for rows.Next() {
		var (
			id                                                                                   int64
			checksum, kind, table, column, statement, priorType, priorName, objectName, priorSQL any
		)

		if err := rows.Scan(
			&id, &checksum, &kind, &table, &column, &statement, &priorType, &priorName, &objectName, &priorSQL,
		); err != nil {
			return nil, fmt.Errorf("[zen/migrate] scan %s: %w", schemaMigrationsTable, err)
		}

		out = append(out, migrationRow{
			ID:         id,
			Checksum:   nullableText(checksum),
			Kind:       nullableText(kind),
			Table:      nullableText(table),
			Column:     nullableText(column),
			Statement:  nullableText(statement),
			PriorType:  nullableText(priorType),
			PriorName:  nullableText(priorName),
			ObjectName: nullableText(objectName),
			PriorSQL:   nullableText(priorSQL),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[zen/migrate] iterate %s: %w", schemaMigrationsTable, err)
	}

	return out, nil
}

// nullableText renders a possibly-NULL text column as a string, tolerating
// the []byte spelling some drivers return for TEXT. NULL becomes "".
func nullableText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}
