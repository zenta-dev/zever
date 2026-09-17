package atlas

// This file renders the same resolved IR as render_schema.go into plain
// SQL DDL, for the `zengo db migrate` bootstrap path.
//
// IMPORTANT — this is NOT Atlas-style migration planning. It emits only
// idempotent "CREATE SCHEMA/TABLE/INDEX IF NOT EXISTS" statements that
// bring an empty database up to the shape the schema describes. It does
// not diff against live database state, never emits ALTER TABLE, never
// drops anything, produces no down migrations, and cannot detect drift.
// A schema change applied to an already-created table is silently a no-op
// here. Driving real Atlas-engine diffing is deliberately deferred.

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// Dialect names accepted by the DDL renderer.
const (
	DialectPostgres = "postgres"
	DialectSQLite   = "sqlite"
	DialectMySQL    = "mysql"
)

// RenderSchemaDDL renders every module of a resolved schema into an
// ordered list of idempotent DDL statements for the named dialect
// ("postgres", "sqlite", or "mysql"), safe to execute in sequence against
// an empty or already-migrated database.
//
// Statement order is CREATE SCHEMA (postgres only), then CREATE TABLE in
// foreign-key dependency order so inline REFERENCES clauses always name an
// already-created table, then CREATE INDEX. Cycles in the foreign-key
// graph fall back to declaration order.
//
// See the file comment: this is a create-only bootstrap, not full
// migration support.
func RenderSchemaDDL(dialect string, schema *ir.Schema) ([]string, error) {
	if err := checkDialect(dialect); err != nil {
		return nil, err
	}

	var stmts []string

	quote := identQuote(dialect)

	if dialect == DialectPostgres {
		for _, enum := range collectNamedEnums(schema) {
			stmts = append(stmts, renderCreateEnumType(quote, enum))
		}
	}

	for _, m := range schema.Modules {
		if dialect == DialectPostgres {
			for _, name := range moduleSchemas(m) {
				stmts = append(stmts, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;", quote(name)))
			}
		}

		ordered := orderEntities(m)
		fks := collectForeignKeys(m)

		for _, e := range ordered {
			stmt, err := renderCreateTable(dialect, e, fks[TableName(e)])
			if err != nil {
				return nil, err
			}

			stmts = append(stmts, stmt)
		}

		for _, e := range ordered {
			stmts = append(stmts, renderCreateIndexes(dialect, e)...)
		}
	}

	return stmts, nil
}

// RenderCreateTableSQL renders one entity as a single idempotent
// "CREATE TABLE IF NOT EXISTS" statement for the named dialect
// ("postgres" or "sqlite"), including its primary key, per-column UNIQUE
// constraints, and inline foreign-key REFERENCES clauses for the
// belongs_to relations the entity itself declares.
//
// Callers that need a whole schema — including cross-entity has_one
// foreign keys, schema creation, and dependency-ordered statements —
// should use RenderSchemaDDL instead.
func RenderCreateTableSQL(dialect string, e *ir.Entity) (string, error) {
	if err := checkDialect(dialect); err != nil {
		return "", err
	}

	table := TableName(e)

	var fks []foreignKey

	for _, r := range e.Relations {
		fk, ok := relationForeignKey(e, r)
		if ok && fk.Table == table {
			fks = append(fks, fk)
		}
	}

	return renderCreateTable(dialect, e, fks)
}

// checkDialect validates a dialect name.
func checkDialect(dialect string) error {
	switch dialect {
	case DialectPostgres, DialectSQLite, DialectMySQL:
		return nil
	default:
		return fmt.Errorf("atlas: unknown SQL dialect %q (want %q, %q, or %q)",
			dialect, DialectPostgres, DialectSQLite, DialectMySQL)
	}
}

// renderCreateTable renders one CREATE TABLE IF NOT EXISTS statement.
func renderCreateTable(dialect string, e *ir.Entity, fks []foreignKey) (string, error) {
	nullable := nullableColumns(fks)
	byColumn := make(map[string]foreignKey, len(fks))

	for _, fk := range fks {
		byColumn[fk.Column] = fk
	}

	parts := make([]string, 0, len(e.Fields)+1)

	for _, f := range e.Fields {
		parts = append(parts, renderColumnDDL(dialect, f, nullable[f.Name] || f.Optional, byColumn))
	}

	if pk := primaryKeyField(e); pk != nil {
		parts = append(parts, fmt.Sprintf("  PRIMARY KEY (%s)", identQuote(dialect)(pk.Name)))
	}

	// MySQL ignores inline column-level REFERENCES clauses (it accepts them
	// syntactically but enforces nothing), so foreign keys are emitted as
	// table-level CONSTRAINT ... FOREIGN KEY clauses there instead -- see
	// renderColumnDDL, which omits the inline form for MySQL.
	if dialect == DialectMySQL {
		for _, fk := range fks {
			parts = append(parts, "  "+renderMySQLForeignKeyConstraint(identQuote(DialectMySQL), fk))
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("atlas: entity %s has no fields, cannot emit CREATE TABLE", e.Name)
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n);",
		qualifiedTable(dialect, e), strings.Join(parts, ",\n")), nil
}

// renderColumnDDL renders one column definition line, including its NOT
// NULL / UNIQUE constraints and any inline REFERENCES clause.
func renderColumnDDL(
	dialect string, f *ir.Field, isNullable bool, byColumn map[string]foreignKey,
) string {
	var b strings.Builder

	quote := identQuote(dialect)

	fmt.Fprintf(&b, "  %s %s", quote(f.Name), sqlColumnType(dialect, f))

	if !isNullable {
		b.WriteString(" NOT NULL")
	}

	if f.Unique && !f.Primary {
		b.WriteString(" UNIQUE")
	}

	if dialect == DialectSQLite && f.Type.Scalar == ir.TEnum {
		fmt.Fprintf(&b, " CHECK (%s IN (%s))", quote(f.Name), strings.Join(quotedEnumValues(f.Type.EnumValues), ", "))
	}

	fk, ok := byColumn[f.Name]
	if ok && dialect != DialectMySQL {
		ref := quote(fk.RefTable)
		if dialect == DialectPostgres {
			ref = quote(fk.RefSchema) + "." + ref
		}

		fmt.Fprintf(&b, " REFERENCES %s (%s)", ref, quote(fk.RefColumn))

		if action := onDeleteSQL(fk.OnDelete); action != "" {
			fmt.Fprintf(&b, " ON DELETE %s", action)
		}
	}

	return b.String()
}

// renderMySQLForeignKeyConstraint renders one table-level
// "CONSTRAINT <name> FOREIGN KEY (...) REFERENCES ..." clause for a MySQL
// CREATE TABLE. MySQL parses but silently ignores inline column-level
// REFERENCES clauses, so a foreign key only actually exists if it is spelled
// as a separate FOREIGN KEY constraint; this is the form the live-diff
// migration engine also emits for an ALTER TABLE ADD CONSTRAINT, so a
// bootstrapped table and a migrated one converge on the same schema.
func renderMySQLForeignKeyConstraint(quote func(string) string, fk foreignKey) string {
	clause := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
		quote(fk.Name), quote(fk.Column), quote(fk.RefTable), quote(fk.RefColumn))

	if action := onDeleteSQL(fk.OnDelete); action != "" {
		clause += " ON DELETE " + action
	}

	return clause
}

// renderCreateIndexes renders the entity's declared index(...) blocks as
// CREATE INDEX / CREATE UNIQUE INDEX statements. Single-field @unique
// columns are not repeated here: renderColumnDDL already emits them as
// inline UNIQUE column constraints.
func renderCreateIndexes(dialect string, e *ir.Entity) []string {
	specs := declaredIndexes(e)
	out := make([]string, 0, len(specs))
	quote := identQuote(dialect)

	for _, spec := range specs {
		columns := make([]string, 0, len(spec.Columns))
		for _, c := range spec.Columns {
			columns = append(columns, quote(c))
		}

		unique := ""
		if spec.Unique {
			unique = "UNIQUE "
		}

		// Postgres and SQLite support CREATE INDEX IF NOT EXISTS; MySQL does
		// not (that is MariaDB), so MySQL gets the bare CREATE INDEX form.
		// The live-diff migration engine only emits a CREATE INDEX after
		// confirming the index is absent live, and this bootstrap path only
		// runs for tables that do not exist yet, so the lack of a guard is
		// safe on the migration path.
		ifNotExists := "IF NOT EXISTS "
		if dialect == DialectMySQL {
			ifNotExists = ""
		}

		out = append(out, fmt.Sprintf("CREATE %sINDEX %s%s ON %s (%s);",
			unique, ifNotExists, quote(spec.Name), qualifiedTable(dialect, e), strings.Join(columns, ", ")))
	}

	return out
}

// qualifiedTable renders an entity's table name, schema-qualified for
// postgres. SQLite and MySQL have no equivalent schema namespace in this
// renderer's scope, so the bare table name is used there and the entity's
// @schema(...) is ignored.
func qualifiedTable(dialect string, e *ir.Entity) string {
	quote := identQuote(dialect)
	table := quote(TableName(e))

	if dialect == DialectPostgres {
		return quote(schemaOf(e)) + "." + table
	}

	return table
}

// quoteIdent double-quotes a SQL identifier, escaping any embedded quote.
// Both Postgres and SQLite accept double-quoted identifiers.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// mysqlQuoteIdent backtick-quotes a SQL identifier for MySQL, escaping any
// embedded backtick.
func mysqlQuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// identQuote returns the identifier-quoting function for dialect: MySQL
// uses backticks, Postgres and SQLite share the double-quote convention.
func identQuote(dialect string) func(string) string {
	if dialect == DialectMySQL {
		return mysqlQuoteIdent
	}

	return quoteIdent
}

// sqlColumnType maps a field's scalar type to a real column type for the
// given dialect.
//
// SQLite has no native uuid, jsonb, timestamptz, or boolean type, so those
// collapse onto its storage classes (TEXT/INTEGER/BLOB) — a lossy but
// standard mapping. TEnum gets a real native type in Postgres (a declared
// enum type, see renderCreateEnumType) and MySQL (inline ENUM(...)); SQLite
// has no native enum at all, so it stays TEXT with a CHECK constraint
// enforcing the value set instead (see renderColumnDDL).
func sqlColumnType(dialect string, f *ir.Field) string {
	switch dialect {
	case DialectSQLite:
		return sqliteColumnType(f)
	case DialectMySQL:
		return mysqlColumnType(f)
	default:
		return postgresColumnType(f)
	}
}

// postgresColumnType maps a scalar to its Postgres column type.
func postgresColumnType(f *ir.Field) string {
	switch f.Type.Scalar {
	case ir.TUUID:
		return "uuid"
	case ir.TString:
		if n, ok := maxLen(f); ok {
			return fmt.Sprintf("varchar(%d)", n)
		}

		return "text"
	case ir.TInt32:
		return "integer"
	case ir.TInt64:
		return "bigint"
	case ir.TFloat32:
		return "real"
	case ir.TFloat64:
		return "double precision"
	case ir.TBool:
		return "boolean"
	case ir.TTimestamp:
		return "timestamptz"
	case ir.TDate:
		return "date"
	case ir.TBytes:
		return "bytea"
	case ir.TJSON:
		return "jsonb"
	case ir.TEnum:
		if f.Type.EnumName != "" {
			return identQuote(DialectPostgres)(enumTypeName(f.Type.EnumName))
		}

		return "text"
	}

	return "text"
}

// enumTypeName returns the Postgres/Atlas type name for a named enum: the
// snake_case form of its declared name (e.g. "TodoStatus" -> "todo_status").
func enumTypeName(name string) string {
	return naming.SnakeCase(name)
}

// quotedEnumValues single-quotes each enum value for embedding in SQL,
// escaping any embedded single quote.
func quotedEnumValues(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}

	return out
}

// collectNamedEnums returns every named ir.Enum across the whole schema, in
// module then declaration order (both already deterministic). Anonymous
// inline enum(...) fields never appear here -- they stay plain TEXT and
// carry no shared type to declare.
func collectNamedEnums(schema *ir.Schema) []*ir.Enum {
	var out []*ir.Enum

	for _, m := range schema.Modules {
		out = append(out, m.Enums...)
	}

	return out
}

// renderCreateEnumType renders one named enum as an idempotent Postgres
// `CREATE TYPE ... AS ENUM (...)` statement. Postgres has no
// `CREATE TYPE IF NOT EXISTS`, so this wraps the statement in a `DO $$`
// block that swallows the "already exists" error on a repeat run -- safe to
// re-execute, and never drops/recreates the type (so existing columns using
// it, and any data in them, are untouched).
func renderCreateEnumType(quote func(string) string, enum *ir.Enum) string {
	return fmt.Sprintf(
		"DO $$ BEGIN\n  CREATE TYPE %s AS ENUM (%s);\nEXCEPTION WHEN duplicate_object THEN null;\nEND $$;",
		quote(enumTypeName(enum.Name)), strings.Join(quotedEnumValues(enum.Values), ", "),
	)
}

// sqliteColumnType maps a scalar to its SQLite column type.
func sqliteColumnType(f *ir.Field) string {
	switch f.Type.Scalar {
	case ir.TUUID:
		return "TEXT"
	case ir.TString:
		if n, ok := maxLen(f); ok {
			return fmt.Sprintf("VARCHAR(%d)", n)
		}

		return "TEXT"
	case ir.TInt32, ir.TInt64:
		return "INTEGER"
	case ir.TFloat32, ir.TFloat64:
		return "REAL"
	case ir.TBool:
		return "INTEGER"
	case ir.TTimestamp, ir.TDate:
		return "TEXT"
	case ir.TBytes:
		return "BLOB"
	case ir.TJSON:
		return "TEXT"
	case ir.TEnum:
		return "TEXT"
	}

	return "TEXT"
}

// mysqlColumnType maps a scalar to its MySQL column type.
//
// A primary-key integer column additionally gets AUTO_INCREMENT, MySQL's
// per-table auto-incrementing-key idiom (there is no sequence type). The
// type-affinity normalizer in orm/migrate strips the AUTO_INCREMENT modifier
// before comparing, since MySQL reports only the bare type in
// information_schema.COLUMN_TYPE and the modifier in the EXTRA column.
//
// TEnum uses inline ENUM(...) directly: MySQL enums are column-local (no
// separate named type to declare), so both named and anonymous enum fields
// render identically here -- only the shared value list matters.
func mysqlColumnType(f *ir.Field) string {
	switch f.Type.Scalar {
	case ir.TUUID:
		return "CHAR(36)"
	case ir.TString:
		if n, ok := maxLen(f); ok {
			return fmt.Sprintf("VARCHAR(%d)", n)
		}

		return "TEXT"
	case ir.TInt32:
		if f.Primary {
			return "INT AUTO_INCREMENT"
		}

		return "INT"
	case ir.TInt64:
		if f.Primary {
			return "BIGINT AUTO_INCREMENT"
		}

		return "BIGINT"
	case ir.TFloat32:
		return "FLOAT"
	case ir.TFloat64:
		return "DOUBLE"
	case ir.TBool:
		// BOOLEAN is a MySQL alias for TINYINT(1); used here for readability.
		return "BOOLEAN"
	case ir.TTimestamp:
		return "DATETIME"
	case ir.TDate:
		return "DATE"
	case ir.TBytes:
		return "BLOB"
	case ir.TJSON:
		// Native JSON requires MySQL 5.7.8+ or MariaDB 10.2.7+.
		return "JSON"
	case ir.TEnum:
		return "ENUM(" + strings.Join(quotedEnumValues(f.Type.EnumValues), ", ") + ")"
	}

	return "TEXT"
}

// orderEntities returns a module's entities in foreign-key dependency
// order: an entity is emitted only after every entity its own belongs_to
// relations reference. Entities caught in a reference cycle (or otherwise
// unresolvable) are appended afterwards in declaration order, which is the
// best a create-only bootstrap can do without ALTER TABLE.
func orderEntities(m *ir.Module) []*ir.Entity {
	deps := make(map[string][]string, len(m.Entities))
	byName := make(map[string]*ir.Entity, len(m.Entities))

	for _, e := range m.Entities {
		byName[e.Name] = e

		for _, r := range e.Relations {
			if r.Kind == ir.BelongsTo && r.Target != nil && r.Target.Name != e.Name {
				deps[e.Name] = append(deps[e.Name], r.Target.Name)
			}
		}
	}

	out := make([]*ir.Entity, 0, len(m.Entities))
	emitted := make(map[string]bool, len(m.Entities))

	for range m.Entities {
		progress := false

		for _, e := range m.Entities {
			if emitted[e.Name] || !depsSatisfied(deps[e.Name], emitted, byName) {
				continue
			}

			emitted[e.Name] = true
			out = append(out, e)
			progress = true
		}

		if !progress {
			break
		}
	}

	for _, e := range m.Entities {
		if !emitted[e.Name] {
			out = append(out, e)
		}
	}

	return out
}

// depsSatisfied reports whether every dependency of an entity has already
// been emitted. A dependency naming an entity outside this module is
// treated as satisfied (the resolver forbids cross-module relations, so
// this is a defensive fallback only).
func depsSatisfied(deps []string, emitted map[string]bool, byName map[string]*ir.Entity) bool {
	for _, d := range deps {
		if _, known := byName[d]; !known {
			continue
		}

		if !emitted[d] {
			return false
		}
	}

	return true
}
