package migrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/dsl/backend/atlas"
)

// This file holds the MySQL half of the live-diffing engine, mirroring the
// SQLite-specific helpers in diff.go and constraints.go: MySQL
// information_schema introspection, MySQL's backtick identifier quoting, and
// the MySQL-correct statement renderers (MODIFY COLUMN, DROP INDEX ... ON
// table) whose syntax differs enough from the Postgres/SQLite path to warrant
// their own functions.
//
// MySQL gaps are fail-closed by design, matching the engine's existing
// posture: the schema_migrations bookkeeping table is rendered with MySQL
// types, and anything MySQL cannot express is either refused with a typed
// error (see renderModifyColumn, which only accepts the mysql dialect) or
// warned-and-skipped, never silently turned into wrong DDL. See doc.go for
// the per-category policy.

// quoteIdent quotes a SQL identifier for the given dialect: backticks for
// MySQL (escaping any embedded backtick), the double-quote convention for
// Postgres and SQLite. atlas.QuoteIdent hardcodes the double-quote form, so
// the MySQL branch is implemented here.
func quoteIdent(dialect, name string) string {
	if dialect == atlas.DialectMySQL {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}

	return atlas.QuoteIdent(name)
}

// mysqlTableExists reports whether an entity's table exists in the connected
// MySQL database. MySQL has no schema namespace in this engine's scope (the
// atlas renderer ignores @schema for MySQL, and the DSN names the database),
// so the check is against DATABASE() rather than the entity's schema.
func mysqlTableExists(ctx context.Context, conn db.DB, table string) (bool, error) {
	rows, err := conn.Query(ctx,
		"SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ? LIMIT 1",
		table)
	if err != nil {
		return false, fmt.Errorf("[orm/migrate] check table %q exists: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	return rows.Next(), nil
}

// introspectMySQLColumns queries information_schema.columns for one table.
// COLUMN_TYPE (not DATA_TYPE) is captured as RawType because it carries the
// unsigned/zerofill/enum-value detail the type normalizer needs to strip or
// compare; DATA_TYPE would lose it. One query per table, mirroring the
// SQLite path: MySQL has no schema-wide batched equivalent the way Postgres
// does (loadLiveSchemaState only batches for postgres).
func introspectMySQLColumns(ctx context.Context, conn db.DB, table string) ([]liveColumn, error) {
	rows, err := conn.Query(ctx,
		"SELECT column_name, column_type, is_nullable, column_default "+
			"FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? "+
			"ORDER BY ordinal_position",
		table)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect columns of %q: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var cols []liveColumn

	for rows.Next() {
		var (
			name       string
			rawType    string
			isNullable string
			colDefault any
		)

		if err := rows.Scan(&name, &rawType, &isNullable, &colDefault); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan column of %q: %w", table, err)
		}

		cols = append(cols, liveColumn{
			Name:       name,
			RawType:    rawType,
			Nullable:   isNullable == "YES",
			HasDefault: colDefault != nil,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] iterate columns of %q: %w", table, err)
	}

	return cols, nil
}

// introspectMySQLIndexes queries information_schema.statistics for every
// non-primary index on one table, grouping the per-column rows into ordered
// liveIndex values by seq_in_index. PRIMARY-key indexes are excluded exactly
// as the Postgres and SQLite paths exclude them: the primary key is diffed as
// part of CREATE TABLE, not as an index.
func introspectMySQLIndexes(ctx context.Context, conn db.DB, table string) ([]liveIndex, error) {
	rows, err := conn.Query(ctx,
		"SELECT index_name, column_name, non_unique "+
			"FROM information_schema.statistics "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND index_name <> 'PRIMARY' "+
			"ORDER BY index_name, seq_in_index",
		table)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect indexes of %q: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	byName := map[string]*liveIndex{}

	var order []string

	for rows.Next() {
		var (
			name      string
			column    string
			nonUnique int
		)

		if err := rows.Scan(&name, &column, &nonUnique); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan index of %q: %w", table, err)
		}

		idx, ok := byName[name]
		if !ok {
			idx = &liveIndex{Name: name, Unique: nonUnique == 0}
			byName[name] = idx
			order = append(order, name)
		}

		idx.Columns = append(idx.Columns, column)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] iterate indexes of %q: %w", table, err)
	}

	out := make([]liveIndex, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}

	return out, nil
}

// introspectMySQLForeignKeys queries information_schema.key_column_usage for
// every foreign-key constraint on one table, matched the same way the
// Postgres/SQLite paths match theirs: by referencing column. Only rows naming
// a referenced table are foreign keys; the view also holds non-constraint
// key usages whose referenced_table_name is NULL.
func introspectMySQLForeignKeys(ctx context.Context, conn db.DB, table string) ([]liveForeignKey, error) {
	rows, err := conn.Query(ctx,
		"SELECT column_name, referenced_table_name, referenced_column_name "+
			"FROM information_schema.key_column_usage "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND referenced_table_name IS NOT NULL "+
			"ORDER BY constraint_name, ordinal_position",
		table)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect foreign keys of %q: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var out []liveForeignKey

	for rows.Next() {
		var fk liveForeignKey
		if err := rows.Scan(&fk.Column, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan foreign key of %q: %w", table, err)
		}

		out = append(out, fk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] iterate foreign keys of %q: %w", table, err)
	}

	return out, nil
}

// renderMySQLDropIndex renders MySQL's DROP INDEX form. MySQL's syntax is
// "DROP INDEX <name> ON <table>" -- there is no schema-qualified index name
// and no IF EXISTS -- unlike Postgres ("DROP INDEX IF EXISTS <qualified>")
// and SQLite ("DROP INDEX IF EXISTS <name>"). The inverse of the stored
// create_index row likewise needs the table, which rollback.go's
// kindCreateIndex branch recovers from row.Table.
func renderMySQLDropIndex(table, name string) string {
	//lint:allow-unsafesql identifier is schema-introspected, not user input
	return fmt.Sprintf("DROP INDEX %s ON %s;", quoteIdent(atlas.DialectMySQL, name), table)
}
