package migrate

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// This file implements the schema-diffing core moved from
// cmd/zengo/migrate_diff.go: column add/drop/type-change detection against a
// live database, plus a schema_migrations bookkeeping table for
// idempotency-by-content. See doc.go for the package-level scope statement
// and cmd/zengo's `zengo db migrate -h` for the user-facing description.
//
// WHAT IS DIFFED:
//
//   - Added columns: a field declared in the schema but absent live becomes
//     one ALTER TABLE ADD COLUMN. Always on.
//   - Dropped columns: a column present live but no longer declared becomes
//     one ALTER TABLE DROP COLUMN, but ONLY under PlanOptions.DropColumns.
//     Dropping a column destroys its data irrecoverably, so it is never
//     silently on.
//   - Type changes: a column whose live type no longer matches the declared
//     one becomes an ALTER COLUMN TYPE on postgres. On sqlite the change is
//     detected and WARNED about, never applied -- see typeaffinity.go.
//   - Renamed columns: a field carrying @renamed_from("old_name") whose old
//     name is still live becomes one ALTER TABLE RENAME COLUMN, and is then
//     excluded from the add and drop passes. A rename cannot be inferred from
//     a live-vs-declared diff (it is indistinguishable from "drop the old
//     name, add the new one"), so the schema must say so explicitly.
//
// Index, foreign-key, unique-constraint, and nullability diffing live in
// constraints.go, following the same "detect against live state, render
// where the dialect allows it, warn where it doesn't" shape this file
// established for type changes.
//
// EXPLICIT NON-GOALS, by design, not by omission:
//
//   - SQLite type changes: sqlite has no ALTER COLUMN TYPE statement at all.
//     Changing a column's type there requires the twelve-step copy-table
//     rebuild, a materially different and riskier code path. This tool
//     detects the change, warns, and skips. Permanent v1 limitation.
//   - Unannotated renames: without @renamed_from the tool has no way to tell
//     a rename from a drop-plus-add, so an unannotated rename still surfaces
//     as one new ADD COLUMN and the old column left alone (or dropped, under
//     PlanOptions.DropColumns). That is a property of diffing, not a gap
//     here.
//   - Rolling back a whole table: rollback.go inverts column-level
//     statements only. A CREATE TABLE is recorded with kind="create_table"
//     and has no inverse, deliberately.

// ErrUnsupportedDialect is returned by Plan (and anything that calls it) when
// the target database's dialect is not one this migration engine can diff
// against live state. Today that is every dialect except postgres, sqlite,
// and mysql -- most notably the Oracle/SQL Server family, which gets neither
// DDL rendering here nor bootstrap support. See doc.go for the full policy
// statement.
var ErrUnsupportedDialect = errors.New("[orm/migrate] dialect not supported for live schema diffing (postgres, sqlite, and mysql only)")

// schemaMigrationsTable is the bookkeeping table name. It records the
// checksum of every DDL statement this tool has successfully executed, so a
// re-run can skip statements it already applied without relying solely on
// each dialect's own "IF NOT EXISTS" support (ALTER TABLE ADD COLUMN has no
// universal IF NOT EXISTS across dialects).
const schemaMigrationsTable = "schema_migrations"

// identPattern is the defense-in-depth allowlist applied to any identifier
// (table name) interpolated directly into a query, used only where the
// underlying driver cannot bind it as a parameter (sqlite's PRAGMA). Table
// names here always come from the compiler's own IR (TableName(entity)),
// never from untrusted external input, but the check costs nothing.
var identPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateIdent defends against interpolating an unexpected identifier into
// a query that cannot bind it as a parameter.
func validateIdent(name string) error {
	if !identPattern.MatchString(name) {
		return fmt.Errorf("[orm/migrate] refusing to use identifier %q (must match %s)", name, identPattern.String())
	}

	return nil
}

// Migration kinds recorded in schema_migrations.kind. Every statement this
// tool executes is tagged with exactly one of these, and inverseStatement
// (rollback.go) dispatches on them to synthesize a rollback.
const (
	kindCreateTable      = "create_table"
	kindAddColumn        = "add_column"
	kindDropColumn       = "drop_column"
	kindRenameColumn     = "rename_column"
	kindAlterType        = "alter_type"
	kindCreateIndex      = "create_index"
	kindDropIndex        = "drop_index"
	kindAddForeignKey    = "add_foreign_key"
	kindAlterNullability = "alter_nullability"
)

// migrationMeta is the bookkeeping recorded alongside a checksum: enough to
// synthesize an inverse statement for rollback.go.
//
// Table holds the ALREADY-QUALIFIED, ALREADY-QUOTED table name exactly as
// atlas.QualifiedTableName rendered it for the forward statement (e.g.
// `"public"."orders"`), so an inverse can be rendered from the row alone
// without re-resolving an ir.Entity that may no longer exist in the schema
// by the time a rollback runs. Column, PriorName and PriorType are raw and
// are validated + quoted at inverse-render time.
type migrationMeta struct {
	Kind      string // one of the kind* constants above
	Table     string
	Column    string
	Statement string
	// PriorType is populated for drop_column/alter_type (the live type
	// BEFORE this ran) and for alter_nullability ("NULL" or "NOT NULL", the
	// nullability state BEFORE this ran).
	PriorType string
	PriorName string // populated only for rename_column: the old column name
	// ObjectName is an already-quoted (and, for indexes on postgres,
	// already schema-qualified) identifier of a named database object this
	// statement created: an index name for create_index/drop_index, or a
	// constraint name for add_foreign_key. Used verbatim by an inverse, the
	// same way Table is.
	ObjectName string
	// PriorSQL is populated only for drop_index: the exact CREATE INDEX
	// statement that recreates the index just dropped, captured at the only
	// moment its column list and uniqueness are still observable.
	PriorSQL string
}

// plannedStatement pairs one DDL statement with the bookkeeping recorded when
// it is applied. The plan is computed as these rather than bare strings so the
// prior state an inverse needs (a dropped column's type, a renamed column's
// old name) is captured at the moment it is still observable -- after the
// statement runs, it is gone.
type plannedStatement struct {
	SQL  string
	Meta migrationMeta
}

// statementTexts flattens a plan to its SQL, for printing.
func statementTexts(stmts []plannedStatement) []string {
	out := make([]string, 0, len(stmts))
	for _, s := range stmts {
		out = append(out, s.SQL)
	}

	return out
}

// PlanOptions carries the caller-selected diffing behaviors into Plan, so
// adding a new one does not mean adding another positional bool everywhere.
type PlanOptions struct {
	// DropColumns enables emitting ALTER TABLE DROP COLUMN for live columns
	// no longer declared in the schema. Off by default: destructive.
	DropColumns bool
	// DetectTypeChanges enables comparing a live column's type against the
	// declared one. On postgres a mismatch emits ALTER COLUMN TYPE; on
	// sqlite it emits a warning and nothing else.
	DetectTypeChanges bool
}

// MigrationPlan is the result of Plan: the ordered DDL statements that would
// bring exec's live schema in line with schema, plus every non-fatal
// diffing warning noticed while computing it, WITHOUT having executed
// anything. Pass it to Apply to actually run it.
type MigrationPlan struct {
	// Dialect is the exec.Dialect() value Plan computed this plan against.
	// Apply uses it to render/ensure the schema_migrations bookkeeping table
	// in the right dialect.
	Dialect string
	// Warnings are non-fatal diffing decisions (an sqlite type/nullability
	// change that was detected but cannot be applied, a foreign key or
	// unique constraint no longer declared but never dropped automatically,
	// etc.), in the order they were noticed. A caller that wants
	// `zengo db migrate`'s exact user-facing behavior should print each one
	// prefixed however it likes -- these strings carry no prefix of their
	// own.
	Warnings []string

	statements []plannedStatement
}

// Statements returns the plan's DDL, in execution order, for previewing
// (e.g. a --dry-run flag) without applying it.
func (p *MigrationPlan) Statements() []string {
	if p == nil {
		return nil
	}

	return statementTexts(p.statements)
}

// Plan computes the ordered list of DDL statements that would bring exec's
// live schema in line with schema, under the rules documented at the top of
// this file, without executing or recording anything.
//
// The dialect is read from exec.Dialect(); only "postgres", "sqlite", and
// "mysql" are supported for live diffing (see ErrUnsupportedDialect and
// doc.go).
func Plan(ctx context.Context, exec db.DB, schema *ir.Schema, opts PlanOptions) (*MigrationPlan, error) {
	dialect := exec.Dialect()

	switch dialect {
	case atlas.DialectPostgres, atlas.DialectSQLite, atlas.DialectMySQL:
	default:
		return nil, fmt.Errorf("[orm/migrate] dialect %q: %w", dialect, ErrUnsupportedDialect)
	}

	w := &warnings{}

	stmts, err := computeMigrationPlan(ctx, exec, dialect, schema, opts, w)
	if err != nil {
		return nil, err
	}

	return &MigrationPlan{Dialect: dialect, Warnings: w.messages(), statements: stmts}, nil
}

// warnings collects non-fatal diffing messages produced while computing a
// Plan, in the order they were noticed, instead of printing them directly --
// this package has no business deciding how a caller formats or displays
// them (cmd/zengo prefixes each one with its own "zengo db migrate: warning:"
// convention).
type warnings struct {
	msgs []string
}

func (w *warnings) add(format string, args ...any) {
	//lint:allow-unsafesql builds a human-readable warning string, not executed SQL
	w.msgs = append(w.msgs, fmt.Sprintf(format, args...))
}

func (w *warnings) messages() []string {
	return w.msgs
}

// tableExists reports whether an entity's table already exists live.
func tableExists(ctx context.Context, conn db.DB, dialect string, e *ir.Entity) (bool, error) {
	table := atlas.TableName(e)

	var rows db.Rows

	var err error

	switch dialect {
	case atlas.DialectPostgres:
		rows, err = conn.Query(ctx,
			"SELECT 1 FROM information_schema.tables WHERE table_schema = ? AND table_name = ? LIMIT 1",
			atlas.SchemaOf(e), table)
	case atlas.DialectSQLite:
		if verr := validateIdent(table); verr != nil {
			return false, verr
		}

		rows, err = conn.Query(ctx, "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", table)
	case atlas.DialectMySQL:
		return mysqlTableExists(ctx, conn, table)
	default:
		return false, fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
	}

	if err != nil {
		return false, fmt.Errorf("[orm/migrate] check table %q exists: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	return rows.Next(), nil
}

// liveColumn is one column's introspected shape from a live database.
type liveColumn struct {
	Name       string
	RawType    string // dialect-native spelling, e.g. "character varying(255)" or "INTEGER"
	Nullable   bool
	HasDefault bool
}

// introspectColumns returns the columns currently present on an entity's
// live table. The caller must already know the table exists.
//
// On Postgres this is a map lookup into state, which loadLiveSchemaState
// populated with ONE schema-wide query per distinct Postgres schema before
// the caller's entity loop began -- see that function's doc comment. On
// SQLite there is no such batching available (SQLite exposes no
// schema-wide equivalent of information_schema; every PRAGMA is inherently
// scoped to one table), so this still issues one PRAGMA table_info(...)
// per call, same as before this optimization.
func introspectColumns(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, state *liveSchemaState,
) ([]liveColumn, error) {
	table := atlas.TableName(e)

	switch dialect {
	case atlas.DialectPostgres:
		return state.columns[table], nil
	case atlas.DialectSQLite:
		return introspectSQLiteColumns(ctx, conn, table)
	case atlas.DialectMySQL:
		return introspectMySQLColumns(ctx, conn, table)
	default:
		return nil, fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
	}
}

// introspectPostgresColumns queries information_schema.columns for every
// table named in tables, in ONE round trip regardless of how many tables
// are named, rather than one round trip per table. The `?` placeholders are
// rewritten to `$1`/`$2` by db/postgres's own placeholder rewriting; the
// pgx driver underneath binds a Go []string directly to `= ANY($n)`.
//
// The result is grouped by table_name in Go into the per-table map the rest
// of the diff logic already expects (liveSchemaState.columns), so callers
// downstream of loadLiveSchemaState never see this batching.
func introspectPostgresColumns(
	ctx context.Context, conn db.DB, schema string, tables []string,
) (map[string][]liveColumn, error) {
	out := make(map[string][]liveColumn, len(tables))

	if len(tables) == 0 {
		return out, nil
	}

	rows, err := conn.Query(ctx,
		"SELECT table_name, column_name, data_type, is_nullable, column_default "+
			"FROM information_schema.columns WHERE table_schema = ? AND table_name = ANY(?)",
		schema, tables)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect columns of schema %q: %w", schema, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var (
			table      string
			name       string
			dataType   string
			isNullable string
			colDefault any
		)

		if err := rows.Scan(&table, &name, &dataType, &isNullable, &colDefault); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan column of schema %q: %w", schema, err)
		}

		out[table] = append(out[table], liveColumn{
			Name:       name,
			RawType:    dataType,
			Nullable:   isNullable == "YES",
			HasDefault: colDefault != nil,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] iterate columns of schema %q: %w", schema, err)
	}

	return out, nil
}

// introspectSQLiteColumns uses PRAGMA table_info(<table>). SQLite's PRAGMA
// statements don't support bind parameters for the table name in all
// driver versions, so the (already-validated) identifier is quoted and
// interpolated directly, matching how internal/dsl/backend/atlas quotes
// identifiers for CREATE TABLE.
func introspectSQLiteColumns(ctx context.Context, conn db.DB, table string) ([]liveColumn, error) {
	if err := validateIdent(table); err != nil {
		return nil, err
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	rows, err := conn.Query(ctx, fmt.Sprintf("PRAGMA table_info(%s)", atlas.QuoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect columns of %q: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var cols []liveColumn

	for rows.Next() {
		// table_info columns: cid, name, type, notnull, dflt_value, pk.
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue any
			pk        int
		)

		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan column of %q: %w", table, err)
		}

		cols = append(cols, liveColumn{
			Name:       name,
			RawType:    colType,
			Nullable:   notNull == 0,
			HasDefault: dfltValue != nil,
		})
	}

	return cols, nil
}

// renderAddColumn renders one ALTER TABLE ADD COLUMN statement for a field
// missing from a live table, reusing atlas's own per-dialect column-type
// mapping and identifier quoting rather than duplicating it.
//
// Postgres supports "ADD COLUMN IF NOT EXISTS" natively (9.6+); SQLite and
// MySQL do not (MySQL's IF NOT EXISTS is a MariaDB extension), so only
// Postgres statements carry it. Either way the statement is still recorded in
// schema_migrations for auditability, per dialect.
func renderAddColumn(dialect string, e *ir.Entity, f *ir.Field) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	colType, err := atlas.ColumnType(dialect, f)
	if err != nil {
		return plannedStatement{}, err
	}

	ifNotExists := ""
	if dialect == atlas.DialectPostgres {
		ifNotExists = "IF NOT EXISTS "
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s%s %s;", table, ifNotExists, quoteIdent(dialect, f.Name), colType)

	return plannedStatement{
		SQL:  sql,
		Meta: migrationMeta{Kind: kindAddColumn, Table: table, Column: f.Name, Statement: sql},
	}, nil
}

// renderDropColumn renders one ALTER TABLE DROP COLUMN statement for a live
// column no longer declared in the schema.
//
// Postgres supports "DROP COLUMN IF EXISTS"; SQLite (3.35+, which the bundled
// modernc driver far exceeds) and MySQL support plain DROP COLUMN only.
//
// priorType is the live column's type as introspected moments ago. It is
// recorded as the statement's prior state because it is the ONLY chance to
// capture it: once the DROP runs, the type is not recoverable from anywhere.
// A rollback re-adds the column with that type -- empty, since the data is
// genuinely gone.
func renderDropColumn(dialect string, e *ir.Entity, column, priorType string) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	if err := validateIdent(column); err != nil {
		return plannedStatement{}, err
	}

	ifExists := ""
	if dialect == atlas.DialectPostgres {
		ifExists = "IF EXISTS "
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s%s;", table, ifExists, quoteIdent(dialect, column))

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindDropColumn, Table: table, Column: column, Statement: sql, PriorType: priorType,
		},
	}, nil
}

// renderRenameColumn renders one ALTER TABLE ... RENAME COLUMN statement.
//
// Unlike ADD/DROP/ALTER TYPE this needs no per-dialect IF guard: postgres,
// sqlite (3.25+, which the bundled modernc driver far exceeds), and MySQL
// (8.0+) all spell it identically, and neither supports an IF EXISTS guard on
// it -- which is exactly why the caller must only emit it when the old name is
// known to be live and the new one is known not to be. (MySQL 5.7 would need
// CHANGE COLUMN instead; the engine targets 8.0+.)
func renderRenameColumn(dialect string, e *ir.Entity, oldName, newName string) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	if err := validateIdent(oldName); err != nil {
		return plannedStatement{}, err
	}

	if err := validateIdent(newName); err != nil {
		return plannedStatement{}, err
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
		table, quoteIdent(dialect, oldName), quoteIdent(dialect, newName))

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindRenameColumn, Table: table, Column: newName, Statement: sql, PriorName: oldName,
		},
	}, nil
}

// renameStatementsFor emits one RENAME COLUMN per field carrying
// @renamed_from("old"), and REWRITES live in place to the post-rename state:
// the old key is removed and the column reappears under the new name.
//
// That in-place rewrite is the whole mechanism by which a rename suppresses
// the drop+add pair it would otherwise decompose into -- the add pass then
// sees the declared name already present, and the drop pass no longer sees an
// undeclared orphan. It also means the type-change pass compares the declared
// field against the right live column, under either name.
//
// A rename is emitted only when the old name is still live AND the new name
// is not: once applied, both conditions fail, so re-running is a no-op even
// though the schema still carries the attribute forever.
func renameStatementsFor(dialect string, e *ir.Entity, live map[string]liveColumn, w *warnings) ([]plannedStatement, error) {
	var stmts []plannedStatement

	// e.Fields is in source-declaration order, so the output is stable
	// without a sort (unlike the drop pass, which iterates a map).
	for _, f := range e.Fields {
		if f.RenamedFrom == nil {
			continue
		}

		old := *f.RenamedFrom

		col, wasLive := live[old]
		if !wasLive {
			continue
		}

		if _, taken := live[f.Name]; taken {
			w.add("%s.%s declares @renamed_from(%q) but both columns exist live; leaving both alone",
				atlas.TableName(e), f.Name, old)

			continue
		}

		stmt, err := renderRenameColumn(dialect, e, old, f.Name)
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)

		delete(live, old)

		col.Name = f.Name
		live[f.Name] = col
	}

	return stmts, nil
}

// dropStatementsFor returns one DROP COLUMN statement per live column absent
// from the entity's declared fields -- the drop-side sibling of the existing
// add-column diff. Output is sorted by column name so a plan is stable across
// runs (live is a map, whose iteration order is not).
//
// Callers must gate this on PlanOptions.DropColumns: every statement it
// returns destroys a column's data irrecoverably.
func dropStatementsFor(dialect string, e *ir.Entity, live map[string]liveColumn) ([]plannedStatement, error) {
	declared := make(map[string]bool, len(e.Fields))
	for _, f := range e.Fields {
		declared[f.Name] = true
	}

	orphans := make([]string, 0, len(live))

	for name := range live {
		if !declared[name] {
			orphans = append(orphans, name)
		}
	}

	sort.Strings(orphans)

	stmts := make([]plannedStatement, 0, len(orphans))

	for _, name := range orphans {
		stmt, err := renderDropColumn(dialect, e, name, live[name].RawType)
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// liveSchemaState holds every existing table's live columns, indexes, and
// foreign keys, fetched by loadLiveSchemaState with a small, CONSTANT number
// of round trips (independent of how many tables the schema declares) rather
// than the four-or-more per-table round trips (existence check, columns,
// indexes, foreign keys -- plus one more per index on SQLite) a naive
// per-entity introspection loop would issue.
//
// On Postgres this is what makes the batching real: one query per category
// (columns / indexes / foreign keys) per distinct Postgres schema, grouped by
// table_name in Go into these maps. On SQLite the maps are left nil/empty --
// SQLite's PRAGMA table_info/index_list/foreign_key_list have no schema-wide
// equivalent (there is no "information_schema" analog to query across every
// table at once), so SQLite introspection stays genuinely per-table and is
// issued lazily, exactly as before this change. See introspectColumns,
// introspectIndexes, and introspectForeignKeys, which each branch on dialect
// to either read from these maps (Postgres) or fall back to a live PRAGMA
// call (SQLite).
type liveSchemaState struct {
	columns     map[string][]liveColumn
	indexes     map[string][]liveIndex
	foreignKeys map[string][]liveForeignKey
}

// loadLiveSchemaState prefetches every declared table's live columns,
// indexes, and foreign keys in one pass, before computeMigrationPlan's
// per-entity loop runs. It is safe to call for tables that do not exist live
// yet (a bootstrap CREATE TABLE case): the batched queries below simply
// return no rows for those table names, which is indistinguishable from "we
// haven't introspected it" for every consumer, since computeMigrationPlan
// only ever consults the maps for entities tableExists already found live.
//
// Tables are grouped by Postgres schema (SchemaOf(e), usually "public" but
// not always -- an entity may declare a different one) so a multi-schema
// project still issues one query per category per SCHEMA, not per table.
func loadLiveSchemaState(ctx context.Context, conn db.DB, dialect string, schema *ir.Schema) (*liveSchemaState, error) {
	state := &liveSchemaState{
		columns:     map[string][]liveColumn{},
		indexes:     map[string][]liveIndex{},
		foreignKeys: map[string][]liveForeignKey{},
	}

	if dialect != atlas.DialectPostgres {
		// SQLite: nothing to prefetch. See liveSchemaState's doc comment.
		return state, nil
	}

	tablesBySchema := map[string][]string{}

	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			s := atlas.SchemaOf(e)
			tablesBySchema[s] = append(tablesBySchema[s], atlas.TableName(e))
		}
	}

	for pgSchema, tables := range tablesBySchema {
		cols, err := introspectPostgresColumns(ctx, conn, pgSchema, tables)
		if err != nil {
			return nil, err
		}

		for table, c := range cols {
			state.columns[table] = c
		}

		idx, err := introspectPostgresIndexes(ctx, conn, pgSchema, tables)
		if err != nil {
			return nil, err
		}

		for table, i := range idx {
			state.indexes[table] = i
		}

		fks, err := introspectPostgresForeignKeys(ctx, conn, pgSchema, tables)
		if err != nil {
			return nil, err
		}

		for table, f := range fks {
			state.foreignKeys[table] = f
		}
	}

	return state, nil
}

// computeMigrationPlan builds the ordered list of DDL statements to bring a
// live database in line with the declared schema, under the rules documented
// at the top of this file:
//
//   - A table that does not exist live at all falls back to the existing
//     bootstrap CREATE TABLE/INDEX rendering, unchanged.
//   - An existing table gets one ALTER TABLE ADD COLUMN per schema column
//     missing live, plus (opt-in) drops and type changes.
func computeMigrationPlan(
	ctx context.Context, conn db.DB, dialect string, schema *ir.Schema, opts PlanOptions, w *warnings,
) ([]plannedStatement, error) {
	var alterStmts []plannedStatement

	// Prefetch live column/index/foreign-key state for every declared table
	// in a small constant number of schema-wide queries (Postgres) before
	// the per-entity loop below runs, instead of each entity below issuing
	// its own live queries. See liveSchemaState's doc comment.
	state, err := loadLiveSchemaState(ctx, conn, dialect, schema)
	if err != nil {
		return nil, err
	}

	missing := &ir.Schema{}

	for _, m := range schema.Modules {
		var missingEntities []*ir.Entity

		for _, e := range m.Entities {
			exists, err := tableExists(ctx, conn, dialect, e)
			if err != nil {
				return nil, err
			}

			if !exists {
				missingEntities = append(missingEntities, e)
				continue
			}

			stmts, err := alterStatementsFor(ctx, conn, dialect, e, opts, state, w)
			if err != nil {
				return nil, err
			}

			alterStmts = append(alterStmts, stmts...)
		}

		if len(missingEntities) > 0 {
			missing.Modules = append(missing.Modules, &ir.Module{Name: m.Name, Entities: missingEntities})
		}
	}

	var stmts []plannedStatement

	if len(missing.Modules) > 0 {
		createStmts, err := atlas.RenderSchemaDDL(dialect, missing)
		if err != nil {
			return nil, err
		}

		// Bootstrap CREATE TABLE/INDEX statements are recorded for
		// auditability but carry no inverse: dropping a whole table that may
		// hold data is far more dangerous than any column-level undo, and is
		// explicitly out of scope for rollback. RenderSchemaDDL flattens
		// tables and indexes into one list, so no per-statement table name
		// is attributed here -- nothing consumes it for this kind.
		for _, s := range createStmts {
			stmts = append(stmts, plannedStatement{
				SQL:  s,
				Meta: migrationMeta{Kind: kindCreateTable, Statement: s},
			})
		}
	}

	return append(stmts, alterStmts...), nil
}

// alterStatementsFor diffs one already-existing table's live shape against
// what the schema declares, in a fixed order:
//
//  1. RENAME COLUMN for every field carrying @renamed_from whose old name is
//     still live (always).
//  2. ADD COLUMN for every declared field absent live (always).
//  3. ALTER COLUMN TYPE for every field whose live type no longer matches
//     (postgres only, opt-in; sqlite warns and skips).
//  4. ALTER COLUMN [SET|DROP] NOT NULL for every field whose declared
//     Optional no longer matches live nullability (postgres only, always on;
//     sqlite warns and skips -- same treatment as type changes, since sqlite
//     has no such statement without a table rebuild either).
//  5. CREATE INDEX / CREATE UNIQUE INDEX for every declared index(...) block
//     or @unique field absent live, and DROP INDEX for every live index this
//     tool recognizes as its own that is no longer declared. Always on: see
//     indexStatementsFor's doc comment for why this is judged safe-by-default.
//  6. ALTER TABLE ADD CONSTRAINT ... FOREIGN KEY for every declared
//     belongs_to/has_one relation missing live (postgres only, always on;
//     sqlite warns and skips -- sqlite cannot add a foreign key constraint to
//     an existing table at all). A live foreign key no longer declared is
//     warned about only, never dropped automatically.
//  7. DROP COLUMN for every live column no longer declared (opt-in only).
//
// Renames come first, and rewrite the live-column map they are computed from,
// so the add and drop passes below never also fire for the two halves of a
// column that is merely changing its name.
//
// Drops come last so a plan that both adds and drops never leaves the table
// momentarily narrower than either schema version describes.
func alterStatementsFor(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, opts PlanOptions, state *liveSchemaState, w *warnings,
) ([]plannedStatement, error) {
	liveCols, err := introspectColumns(ctx, conn, dialect, e, state)
	if err != nil {
		return nil, err
	}

	live := make(map[string]liveColumn, len(liveCols))
	for _, c := range liveCols {
		live[c.Name] = c
	}

	stmts, err := renameStatementsFor(dialect, e, live, w)
	if err != nil {
		return nil, err
	}

	forcedNullable := atlas.NullableColumns(atlas.EntityForeignKeys(e))

	for _, f := range e.Fields {
		col, ok := live[f.Name]
		if !ok {
			stmt, aerr := renderAddColumn(dialect, e, f)
			if aerr != nil {
				return nil, aerr
			}

			stmts = append(stmts, stmt)

			continue
		}

		if opts.DetectTypeChanges {
			stmt, terr := typeChangeStatementFor(dialect, e, f, col, forcedNullable[f.Name], w)
			if terr != nil {
				return nil, terr
			}

			if stmt.SQL != "" {
				stmts = append(stmts, stmt)
			}
		}

		nstmt, nerr := nullabilityStatementFor(dialect, e, f, col, forcedNullable[f.Name], w)
		if nerr != nil {
			return nil, nerr
		}

		if nstmt.SQL != "" {
			stmts = append(stmts, nstmt)
		}
	}

	idxStmts, err := indexStatementsFor(ctx, conn, dialect, e, state, w)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, idxStmts...)

	fkStmts, err := foreignKeyStatementsFor(ctx, conn, dialect, e, state, w)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, fkStmts...)

	if opts.DropColumns {
		drops, derr := dropStatementsFor(dialect, e, live)
		if derr != nil {
			return nil, derr
		}

		stmts = append(stmts, drops...)
	}

	return stmts, nil
}

// typeChangeStatementFor returns the ALTER COLUMN TYPE / MODIFY COLUMN
// statement for a field whose live type no longer matches its declared one,
// or a zero-value plannedStatement when there is nothing to do.
//
// On postgres the change renders ALTER COLUMN TYPE; on mysql it renders
// MODIFY COLUMN (see renderModifyColumn). On sqlite a detected change yields
// nothing plus a warning: sqlite has no ALTER COLUMN TYPE at all, so the only
// honest options are "warn and skip" or the out-of-scope table-rebuild dance.
// Silently doing nothing would be worse than either.
//
// forcedNullable mirrors nullabilityStatementFor's parameter: a MODIFY COLUMN
// on mysql must restate the whole column definition, including nullability,
// so the declared nullability (with the @on_delete(set_null) exception
// applied) is folded in rather than left to the database's MODIFY default.
func typeChangeStatementFor(
	dialect string, e *ir.Entity, f *ir.Field, col liveColumn, forcedNullable bool, w *warnings,
) (plannedStatement, error) {
	changed, err := typeChanged(dialect, f, col)
	if err != nil {
		return plannedStatement{}, err
	}

	if !changed {
		return plannedStatement{}, nil
	}

	switch dialect {
	case atlas.DialectPostgres:
		return renderAlterColumnType(dialect, e, f, col.RawType)
	case atlas.DialectMySQL:
		nullable := f.Optional || forcedNullable

		return renderModifyColumn(dialect, e, f, nullable, kindAlterType, mysqlColumnDefinition(col))
	default:
		w.add("type change detected on %s.%s (live %q) but %s does not support ALTER COLUMN TYPE; skipping",
			atlas.TableName(e), f.Name, col.RawType, dialect)

		return plannedStatement{}, nil
	}
}
