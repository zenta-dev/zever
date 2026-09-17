package migrate

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// This file extends diff.go's diffing past columns to indexes, unique
// constraints, foreign keys, and nullability. Moved from
// orm/migrate/constraints.go.
//
// WHAT IS DIFFED AND APPLIED, PER DIALECT:
//
//   - Declared index(...) blocks: added when missing live, dropped when a
//     live index THIS TOOL RECOGNIZES AS ITS OWN (see looksManaged) is no
//     longer declared. Always on, on postgres, sqlite, and mysql: creating or
//     dropping an index changes no data, only a query-performance/uniqueness
//     guarantee, so there is nothing here worth gating behind a flag. An
//     index this tool did not create (no matching name pattern) is left
//     alone even if undeclared -- a hand-added DBA index must never be
//     silently dropped by a tool that does not know it exists for a reason.
//   - @unique fields: treated as an implied single-column UNIQUE INDEX (the
//     same object SQLite itself uses to enforce a UNIQUE column constraint
//     under the hood -- see https://sqlite.org/lang_createtable.html), named
//     with the same "<table>_<column>_key" convention entityIndexes uses for
//     Atlas HCL rendering. A missing one is added on ALL THREE dialects via
//     CREATE UNIQUE INDEX, which needs no table rebuild on any. A live one
//     no longer declared is WARNED about only, never dropped: on postgres an
//     inline `UNIQUE` column constraint is backed by a genuine pg_constraint
//     entry, not a bare index, and dropping it needs ALTER TABLE DROP
//     CONSTRAINT with the RIGHT constraint name -- which requires additional
//     per-dialect introspection (pg_constraint) this v1 does not add. Treated
//     the same as a live foreign key no longer declared, for the same reason:
//     an automatic DROP CONSTRAINT is a bigger commitment than an automatic
//     DROP INDEX, so it stays manual.
//   - Foreign keys (derived from belongs_to/has_one exactly as
//     atlas.CollectForeignKeys derives them for bootstrap rendering): a
//     missing declared one is added via ALTER TABLE ADD CONSTRAINT ...
//     FOREIGN KEY ON POSTGRES AND MYSQL. SQLite cannot add a foreign key
//     constraint to an existing table under any syntax -- this is a
//     documented SQLite limitation, not an oversight -- so sqlite gets a
//     detect-and-warn treatment identical to typeaffinity.go's handling of
//     sqlite type changes. A live foreign key no longer declared is warned
//     about only, never dropped, on any dialect: for the same "which
//     constraint, exactly" reason as the unique-constraint case above,
//     compounded by sqlite being unable to drop one without a table rebuild
//     even if it wanted to.
//   - Nullability (ir.Field.Optional vs. the live column's NOT NULL): a
//     mismatch renders ALTER COLUMN [SET|DROP] NOT NULL ON POSTGRES, and
//     MODIFY COLUMN ON MYSQL (see renderModifyColumn, which must restate the
//     whole column). SQLite has no such statement without the same
//     twelve-step table rebuild type changes would need, so it gets the
//     identical detect-and-warn treatment. A field forced nullable by an
//     @on_delete(set_null) foreign key (see atlas.NullableColumns) is
//     excluded from the comparison the same way bootstrap rendering excludes
//     it, so that mechanism never looks like drift.
//
// EVERY new statement kind flows through the SAME migrationMeta/
// plannedStatement/recordMigration mechanism diff.go already established, so
// rollback.go inverts them for free -- see that file for what each kind's
// inverse looks like and which ones are lossy.

// liveIndex is one index's introspected shape from a live database:
// PRIMARY KEY-backing indexes are never returned (both introspection
// functions filter them out), since the primary key is diffed as part of
// CREATE TABLE, not as an index.
type liveIndex struct {
	Name    string
	Columns []string
	Unique  bool
}

// liveForeignKey is one foreign-key constraint's introspected shape from a
// live database.
type liveForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// managedIndexPattern matches the two index-naming conventions this tool
// itself generates (see atlas.DeclaredIndexes and uniqueIndexName): a
// "<table>_..._idx" for a plain declared index(...) block, or
// "<table>_..._key" for a unique one. Used by looksManaged to decide whether
// an undeclared live index is safe to auto-drop.
func managedIndexPattern(table string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(table) + `_.+_(idx|key)$`)
}

// looksManaged reports whether an index name matches this tool's own naming
// convention for a table, so the automatic DROP INDEX pass never touches an
// index it did not create -- a hand-added DBA index, or (on sqlite) the
// engine's own "sqlite_autoindex_..." backing an inline UNIQUE column
// constraint, which this tool tracks separately as a unique-field diff, not
// an index() block diff.
func looksManaged(name, table string) bool {
	return managedIndexPattern(table).MatchString(name)
}

// uniqueIndexName returns the name this tool uses for the unique index that
// enforces an @unique field, matching entityIndexes's convention exactly so
// a table bootstrapped via RenderSchemaDDL and one migrated into shape here
// converge on the same identifier.
func uniqueIndexName(table, column string) string {
	//lint:allow-unsafesql identifier is schema-introspected, not user input
	return fmt.Sprintf("%s_%s_key", table, column)
}

// createIndexPattern is the defense-in-depth allowlist applied to a
// rollback.go inverse's prior_sql before it is executed: it must be a
// statement this tool itself generated (see createIndexSQL), never
// arbitrary SQL from a hand-edited row. The "IF NOT EXISTS" guard is
// optional because MySQL has no such clause and its CREATE INDEX statements
// (and therefore their captured PriorSQL) omit it.
var createIndexPattern = regexp.MustCompile(`^CREATE (UNIQUE )?INDEX (IF NOT EXISTS )?`)

// createIndexSQL renders one CREATE INDEX statement, shared by
// renderCreateIndex (a genuinely missing index) and renderDropIndex (which
// needs the exact same text as the PriorSQL a rollback replays).
//
// Postgres and SQLite support "IF NOT EXISTS"; MySQL does not, so MySQL omits
// it -- the caller has already confirmed the index is absent live, and the
// plan's checksum dedup in schema_migrations makes a re-run a no-op either
// way.
func createIndexSQL(dialect, table, name string, columns []string, unique bool) (string, error) {
	if err := validateIdent(name); err != nil {
		return "", err
	}

	cols := make([]string, len(columns))

	for i, c := range columns {
		if err := validateIdent(c); err != nil {
			return "", err
		}

		cols[i] = quoteIdent(dialect, c)
	}

	uniqueKW := ""
	if unique {
		uniqueKW = "UNIQUE "
	}

	ifNotExists := "IF NOT EXISTS "
	if dialect == atlas.DialectMySQL {
		ifNotExists = ""
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	return fmt.Sprintf("CREATE %sINDEX %s%s ON %s (%s);",
		uniqueKW, ifNotExists, quoteIdent(dialect, name), table, strings.Join(cols, ", ")), nil
}

// qualifiedIndexName returns an index's identifier, schema-qualified for
// Postgres (DROP INDEX accepts a schema-qualified name; CREATE INDEX does
// not, which is why createIndexSQL never calls this). SQLite and MySQL have
// no such namespace, so the bare quoted name is used there.
func qualifiedIndexName(dialect string, e *ir.Entity, name string) string {
	if dialect == atlas.DialectPostgres {
		return quoteIdent(dialect, atlas.SchemaOf(e)) + "." + quoteIdent(dialect, name)
	}

	return quoteIdent(dialect, name)
}

// renderCreateIndex renders one CREATE INDEX/CREATE UNIQUE INDEX statement
// for a declared index missing from a live table.
func renderCreateIndex(dialect string, e *ir.Entity, spec atlas.IndexSpec) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	sql, err := createIndexSQL(dialect, table, spec.Name, spec.Columns, spec.Unique)
	if err != nil {
		return plannedStatement{}, err
	}

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindCreateIndex, Table: table, Statement: sql,
			ObjectName: qualifiedIndexName(dialect, e, spec.Name),
		},
	}, nil
}

// renderDropIndex renders one DROP INDEX statement for a live index this
// tool recognizes as its own that is no longer declared, capturing the
// index's current shape as PriorSQL so a rollback can recreate it exactly.
//
// MySQL spells the statement "DROP INDEX <name> ON <table>" with no IF
// EXISTS; Postgres and SQLite spell it "DROP INDEX [IF EXISTS] <name>".
func renderDropIndex(dialect string, e *ir.Entity, live liveIndex) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	if identErr := validateIdent(live.Name); identErr != nil {
		return plannedStatement{}, identErr
	}

	var sql string
	if dialect == atlas.DialectMySQL {
		sql = renderMySQLDropIndex(table, live.Name)
	} else {
		qualified := qualifiedIndexName(dialect, e, live.Name)
		//lint:allow-unsafesql identifier is schema-introspected, not user input
		sql = fmt.Sprintf("DROP INDEX IF EXISTS %s;", qualified)
	}

	priorSQL, err := createIndexSQL(dialect, table, live.Name, live.Columns, live.Unique)
	if err != nil {
		return plannedStatement{}, err
	}

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindDropIndex, Table: table, Statement: sql,
			ObjectName: qualifiedIndexName(dialect, e, live.Name), PriorSQL: priorSQL,
		},
	}, nil
}

// introspectIndexes returns every non-primary-key index currently on an
// entity's live table.
//
// On Postgres this is a map lookup into state, prefetched schema-wide by
// loadLiveSchemaState (see diff.go). On SQLite there is no schema-wide
// equivalent to batch against (see liveSchemaState's doc comment), so this
// still queries live, one table at a time, same as before.
func introspectIndexes(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, state *liveSchemaState,
) ([]liveIndex, error) {
	table := atlas.TableName(e)

	switch dialect {
	case atlas.DialectPostgres:
		return state.indexes[table], nil
	case atlas.DialectSQLite:
		return introspectSQLiteIndexes(ctx, conn, table)
	case atlas.DialectMySQL:
		return introspectMySQLIndexes(ctx, conn, table)
	default:
		return nil, fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
	}
}

// introspectPostgresIndexes queries the system catalogs directly
// (pg_class/pg_index/pg_attribute) rather than the pg_indexes view, so the
// exact ordered column list of a multi-column index is available -- the
// indexdef text pg_indexes offers is not reliably parseable back into a
// column list. Primary-key-backing indexes are excluded: the primary key is
// diffed as part of the column pass, not here.
//
// tables is queried in ONE round trip via `t.relname = ANY(?)`, regardless
// of how many tables are named, and the result is grouped by table name in
// Go into the per-table map loadLiveSchemaState assembles into
// liveSchemaState.indexes.
func introspectPostgresIndexes(ctx context.Context, conn db.DB, schema string, tables []string) (map[string][]liveIndex, error) {
	out := make(map[string][]liveIndex, len(tables))

	if len(tables) == 0 {
		return out, nil
	}

	rows, err := conn.Query(ctx, `
SELECT t.relname AS table_name, ix.relname AS index_name, a.attname AS column_name, i.indisunique
FROM pg_class t
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_index i ON i.indrelid = t.oid
JOIN pg_class ix ON ix.oid = i.indexrelid
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(i.indkey)
WHERE t.relname = ANY(?) AND n.nspname = ? AND NOT i.indisprimary
ORDER BY t.relname, ix.relname, array_position(i.indkey, a.attnum)`, tables, schema)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect indexes of schema %q: %w", schema, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	byTableAndName := map[string]map[string]*liveIndex{}
	orderByTable := map[string][]string{}

	for rows.Next() {
		var (
			table  string
			name   string
			column string
			unique bool
		)

		if err := rows.Scan(&table, &name, &column, &unique); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan index of schema %q: %w", schema, err)
		}

		byName, ok := byTableAndName[table]
		if !ok {
			byName = map[string]*liveIndex{}
			byTableAndName[table] = byName
		}

		idx, ok := byName[name]
		if !ok {
			idx = &liveIndex{Name: name, Unique: unique}
			byName[name] = idx

			orderByTable[table] = append(orderByTable[table], name)
		}

		idx.Columns = append(idx.Columns, column)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] iterate indexes of schema %q: %w", schema, err)
	}

	for table, order := range orderByTable {
		byName := byTableAndName[table]

		list := make([]liveIndex, 0, len(order))
		for _, name := range order {
			list = append(list, *byName[name])
		}

		out[table] = list
	}

	return out, nil
}

// introspectSQLiteIndexes uses PRAGMA index_list(table) plus one
// PRAGMA index_info(name) per index for its column list. Indexes with
// origin="pk" (the implicit index backing a non-INTEGER PRIMARY KEY) are
// excluded, matching the postgres path's exclusion of primary-key indexes.
func introspectSQLiteIndexes(ctx context.Context, conn db.DB, table string) ([]liveIndex, error) {
	if err := validateIdent(table); err != nil {
		return nil, err
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	rows, err := conn.Query(ctx, fmt.Sprintf("PRAGMA index_list(%s)", atlas.QuoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect indexes of %q: %w", table, err)
	}

	type rawIndex struct {
		name   string
		unique bool
	}

	var raw []rawIndex

	for rows.Next() {
		// index_list columns: seq, name, unique, origin, partial.
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)

		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("[orm/migrate] scan index_list of %q: %w", table, err)
		}

		if origin == "pk" {
			continue
		}

		raw = append(raw, rawIndex{name: name, unique: unique == 1})
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("[orm/migrate] close index_list of %q: %w", table, err)
	}

	out := make([]liveIndex, 0, len(raw))

	for _, r := range raw {
		cols, err := introspectSQLiteIndexColumns(ctx, conn, r.name)
		if err != nil {
			return nil, err
		}

		out = append(out, liveIndex{Name: r.name, Columns: cols, Unique: r.unique})
	}

	return out, nil
}

// introspectSQLiteIndexColumns returns one index's column names in order via
// PRAGMA index_info(name).
//
// This is genuinely one query per index, on top of the one query per table
// introspectSQLiteIndexes already issues for PRAGMA index_list. SQLite has
// no documented PRAGMA that reports every index's column list across a whole
// table (let alone a whole database) in one call -- index_info is
// necessarily scoped to a single named index -- so unlike the Postgres path
// in this file, this cannot be collapsed into a single schema-wide query.
// All of these PRAGMA calls do at least already share the one *db.DB
// connection passed down from computeMigrationPlan (conn), so there is no
// separate per-call connection-acquisition cost to remove here; the
// per-table/per-index round trips themselves are the real, unavoidable
// cost, and are cheap in practice because SQLite is typically embedded/local
// rather than accessed over a network.
func introspectSQLiteIndexColumns(ctx context.Context, conn db.DB, indexName string) ([]string, error) {
	if err := validateIdent(indexName); err != nil {
		return nil, err
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	rows, err := conn.Query(ctx, fmt.Sprintf("PRAGMA index_info(%s)", atlas.QuoteIdent(indexName)))
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect index_info of %q: %w", indexName, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var cols []string

	for rows.Next() {
		// index_info columns: seqno, cid, name.
		var (
			seqno int
			cid   int
			name  any
		)

		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan index_info of %q: %w", indexName, err)
		}

		if s, ok := name.(string); ok {
			cols = append(cols, s)
		}
	}

	return cols, nil
}

// indexStatementsFor diffs one table's live indexes against its declared
// index(...) blocks and @unique fields. See this file's doc comment for the
// exact add/drop rules per category.
func indexStatementsFor(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, state *liveSchemaState, w *warnings,
) ([]plannedStatement, error) {
	liveIdx, err := introspectIndexes(ctx, conn, dialect, e, state)
	if err != nil {
		return nil, err
	}

	diff := newIndexDiff(e, liveIdx)

	var stmts []plannedStatement

	created, err := diff.createDeclaredIndexes(dialect, e)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, created...)

	createdUnique, err := diff.createUniqueFieldIndexes(dialect, e)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, createdUnique...)

	dropped, err := diff.dropManagedIndexes(dialect, e)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, dropped...)

	diff.warnUndeclaredUniqueConstraints(w)

	return stmts, nil
}

// indexDiff holds the declared-vs-live index state for one entity, computed
// once and shared by indexStatementsFor's four passes (see this file's doc
// comment for what each pass does and why).
type indexDiff struct {
	table          string
	liveByName     map[string]liveIndex
	liveUniqueCols map[string]bool
	declaredByName map[string]atlas.IndexSpec
	// declaredUniqueCols is every column covered by a still-declared unique
	// constraint, whether via @unique or a single-column unique index(...)
	// block, so the warn pass never flags one as newly-undeclared.
	declaredUniqueCols map[string]bool
	// uniqueFieldNames is the set of unique-field-implied index names this
	// entity currently declares, so the drop pass never treats one as an
	// orphaned index(...) block just because it is not in declaredByName.
	uniqueFieldNames map[string]bool
}

func newIndexDiff(e *ir.Entity, liveIdx []liveIndex) *indexDiff {
	d := &indexDiff{
		table:              atlas.TableName(e),
		liveByName:         make(map[string]liveIndex, len(liveIdx)),
		liveUniqueCols:     make(map[string]bool),
		declaredUniqueCols: make(map[string]bool),
		uniqueFieldNames:   make(map[string]bool),
	}

	for _, li := range liveIdx {
		d.liveByName[li.Name] = li
		if li.Unique && len(li.Columns) == 1 {
			d.liveUniqueCols[li.Columns[0]] = true
		}
	}

	declared := atlas.DeclaredIndexes(e)
	d.declaredByName = make(map[string]atlas.IndexSpec, len(declared))

	for _, s := range declared {
		d.declaredByName[s.Name] = s

		if s.Unique && len(s.Columns) == 1 {
			d.declaredUniqueCols[s.Columns[0]] = true
		}
	}

	for _, f := range e.Fields {
		if f.Unique && !f.Primary {
			d.declaredUniqueCols[f.Name] = true
			d.uniqueFieldNames[uniqueIndexName(d.table, f.Name)] = true
		}
	}

	return d
}

// createDeclaredIndexes returns CREATE INDEX/CREATE UNIQUE INDEX for every
// declared index(...) block missing live.
func (d *indexDiff) createDeclaredIndexes(dialect string, e *ir.Entity) ([]plannedStatement, error) {
	var names []string

	for name := range d.declaredByName {
		if _, ok := d.liveByName[name]; !ok {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	stmts := make([]plannedStatement, 0, len(names))

	for _, name := range names {
		stmt, err := renderCreateIndex(dialect, e, d.declaredByName[name])
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// createUniqueFieldIndexes returns CREATE UNIQUE INDEX for every @unique
// field not already enforced by some live single-column unique index,
// regardless of that index's own name.
func (d *indexDiff) createUniqueFieldIndexes(dialect string, e *ir.Entity) ([]plannedStatement, error) {
	var stmts []plannedStatement

	for _, f := range e.Fields {
		if !f.Unique || f.Primary || d.liveUniqueCols[f.Name] {
			continue
		}

		spec := atlas.IndexSpec{Name: uniqueIndexName(d.table, f.Name), Columns: []string{f.Name}, Unique: true}

		stmt, err := renderCreateIndex(dialect, e, spec)
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// dropManagedIndexes returns DROP INDEX for every live index this tool
// recognizes as its own (looksManaged) that is neither a still-declared
// index(...) block nor a still-declared unique field's implied index.
func (d *indexDiff) dropManagedIndexes(dialect string, e *ir.Entity) ([]plannedStatement, error) {
	var names []string

	for name := range d.liveByName {
		if _, ok := d.declaredByName[name]; ok {
			continue
		}

		if d.uniqueFieldNames[name] || !looksManaged(name, d.table) {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	stmts := make([]plannedStatement, 0, len(names))

	for _, name := range names {
		stmt, err := renderDropIndex(dialect, e, d.liveByName[name])
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// warnUndeclaredUniqueConstraints warns (never drops) about a live
// single-column unique index whose column is no longer declared unique --
// see this file's doc comment for why this stays manual.
func (d *indexDiff) warnUndeclaredUniqueConstraints(w *warnings) {
	var cols []string

	for col := range d.liveUniqueCols {
		if !d.declaredUniqueCols[col] {
			cols = append(cols, col)
		}
	}

	sort.Strings(cols)

	for _, col := range cols {
		w.add(
			"live unique constraint on %s.%s is no longer declared; dropping a unique constraint is not "+
				"attempted automatically (see package doc)",
			d.table, col)
	}
}

// onDeleteClause maps the DSL's @on_delete value to the SQL clause used in
// an ADD CONSTRAINT ... FOREIGN KEY statement, mirroring atlas's own
// (unexported) onDeleteSQL rather than exporting it for one three-case use.
func onDeleteClause(v string) string {
	switch v {
	case "restrict":
		return " ON DELETE RESTRICT"
	case "cascade":
		return " ON DELETE CASCADE"
	case "set_null":
		return " ON DELETE SET NULL"
	default:
		return ""
	}
}

// renderAddForeignKey renders one ALTER TABLE ADD CONSTRAINT ... FOREIGN KEY
// statement. Postgres and MySQL only: sqlite cannot add a foreign key
// constraint to an existing table under any syntax, so callers must warn and
// skip on sqlite rather than call this, exactly as typeChangeStatementFor
// does for type changes.
//
// MySQL references a bare table name (the atlas renderer ignores @schema for
// MySQL, so there is no schema qualifier to emit), and its constraint name is
// recorded so a rollback can issue MySQL's DROP FOREIGN KEY.
func renderAddForeignKey(dialect string, e *ir.Entity, fk atlas.ForeignKey) (plannedStatement, error) {
	if dialect != atlas.DialectPostgres && dialect != atlas.DialectMySQL {
		return plannedStatement{}, fmt.Errorf(
			"[orm/migrate] dialect %q cannot add a foreign key constraint to an existing table", dialect)
	}

	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	if err := validateIdent(fk.Column); err != nil {
		return plannedStatement{}, err
	}

	if err := validateIdent(fk.Name); err != nil {
		return plannedStatement{}, err
	}

	ref := quoteIdent(dialect, fk.RefTable)
	if dialect == atlas.DialectPostgres {
		ref = quoteIdent(dialect, fk.RefSchema) + "." + ref
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s;",
		table, quoteIdent(dialect, fk.Name), quoteIdent(dialect, fk.Column), ref, quoteIdent(dialect, fk.RefColumn),
		onDeleteClause(fk.OnDelete))

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindAddForeignKey, Table: table, Column: fk.Column, Statement: sql,
			ObjectName: quoteIdent(dialect, fk.Name),
		},
	}, nil
}

// introspectForeignKeys returns the foreign-key constraints currently on an
// entity's live table.
//
// On Postgres this is a map lookup into state, prefetched schema-wide by
// loadLiveSchemaState. On SQLite there is no schema-wide equivalent (see
// liveSchemaState's doc comment), so this still queries live per table.
func introspectForeignKeys(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, state *liveSchemaState,
) ([]liveForeignKey, error) {
	table := atlas.TableName(e)

	switch dialect {
	case atlas.DialectPostgres:
		return state.foreignKeys[table], nil
	case atlas.DialectSQLite:
		return introspectSQLiteForeignKeys(ctx, conn, table)
	case atlas.DialectMySQL:
		return introspectMySQLForeignKeys(ctx, conn, table)
	default:
		return nil, fmt.Errorf("[orm/migrate] unsupported dialect %q", dialect)
	}
}

// introspectPostgresForeignKeys joins the three information_schema views
// needed to recover a foreign key's referencing column and referenced
// table/column: table_constraints (which rows are foreign keys),
// key_column_usage (the referencing column), and constraint_column_usage
// (the referenced table/column).
//
// tables is queried in ONE round trip via `tc.table_name = ANY(?)`, and the
// result is grouped by table name in Go into the per-table map
// loadLiveSchemaState assembles into liveSchemaState.foreignKeys.
func introspectPostgresForeignKeys(
	ctx context.Context, conn db.DB, schema string, tables []string,
) (map[string][]liveForeignKey, error) {
	out := make(map[string][]liveForeignKey, len(tables))

	if len(tables) == 0 {
		return out, nil
	}

	rows, err := conn.Query(ctx, `
SELECT tc.table_name, kcu.column_name, ccu.table_name, ccu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = ? AND tc.table_name = ANY(?)`, schema, tables)
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect foreign keys of schema %q: %w", schema, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var table string

		var fk liveForeignKey
		if err := rows.Scan(&table, &fk.Column, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan foreign key of schema %q: %w", schema, err)
		}

		out[table] = append(out[table], fk)
	}

	return out, nil
}

// introspectSQLiteForeignKeys uses PRAGMA foreign_key_list(table).
func introspectSQLiteForeignKeys(ctx context.Context, conn db.DB, table string) ([]liveForeignKey, error) {
	if err := validateIdent(table); err != nil {
		return nil, err
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	rows, err := conn.Query(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", atlas.QuoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("[orm/migrate] introspect foreign keys of %q: %w", table, err)
	}

	defer func() {
		_ = rows.Close()
	}()

	var out []liveForeignKey

	for rows.Next() {
		// foreign_key_list columns: id, seq, table, from, to, on_update,
		// on_delete, match.
		var (
			id, seq                       int
			refTable, from, to            string
			onUpdate, onDelete, matchType any
		)

		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &matchType); err != nil {
			return nil, fmt.Errorf("[orm/migrate] scan foreign_key_list of %q: %w", table, err)
		}

		out = append(out, liveForeignKey{Column: from, RefTable: refTable, RefColumn: to})
	}

	return out, nil
}

// foreignKeyStatementsFor diffs one table's live foreign keys against the
// ones its belongs_to/has_one relations declare, matched by referencing
// column (not constraint name, since a live constraint's database-assigned
// name need not match the one this tool would have chosen).
func foreignKeyStatementsFor(
	ctx context.Context, conn db.DB, dialect string, e *ir.Entity, state *liveSchemaState, w *warnings,
) ([]plannedStatement, error) {
	liveFKs, err := introspectForeignKeys(ctx, conn, dialect, e, state)
	if err != nil {
		return nil, err
	}

	liveByCol := make(map[string]liveForeignKey, len(liveFKs))
	for _, fk := range liveFKs {
		liveByCol[fk.Column] = fk
	}

	declared := atlas.EntityForeignKeys(e)
	declaredByCol := make(map[string]atlas.ForeignKey, len(declared))

	for _, fk := range declared {
		declaredByCol[fk.Column] = fk
	}

	table := atlas.TableName(e)

	var stmts []plannedStatement

	var addCols []string

	for col := range declaredByCol {
		if _, ok := liveByCol[col]; !ok {
			addCols = append(addCols, col)
		}
	}

	sort.Strings(addCols)

	for _, col := range addCols {
		fk := declaredByCol[col]

		if dialect != atlas.DialectPostgres && dialect != atlas.DialectMySQL {
			w.add(
				"foreign key %s.%s -> %s.%s is declared but missing live; %s cannot add a foreign key constraint to an existing table, skipping",
				table, col, fk.RefTable, fk.RefColumn, dialect)

			continue
		}

		stmt, err := renderAddForeignKey(dialect, e, fk)
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, stmt)
	}

	var dropCols []string

	for col := range liveByCol {
		if _, ok := declaredByCol[col]; !ok {
			dropCols = append(dropCols, col)
		}
	}

	sort.Strings(dropCols)

	for _, col := range dropCols {
		w.add(
			"live foreign key on %s.%s is no longer declared; dropping a foreign key constraint is not attempted automatically (see package doc)",
			table, col)
	}

	return stmts, nil
}

// nullabilityStatementFor returns the statement for a field whose declared
// Optional no longer matches a live column's nullability, or a zero-value
// plannedStatement when there is nothing to do.
//
// forcedNullable is true when an @on_delete(set_null) foreign key forces
// this column nullable regardless of Optional (see atlas.NullableColumns) --
// the same exception bootstrap rendering already applies, so it must not
// look like drift here.
//
// On postgres the change renders ALTER COLUMN [SET|DROP] NOT NULL; on mysql
// it renders MODIFY COLUMN, which must restate the whole column definition
// (see renderModifyColumn). On sqlite the change is detected and warned
// about but never applied: sqlite has no such statement without the
// twelve-step table rebuild.
func nullabilityStatementFor(
	dialect string, e *ir.Entity, f *ir.Field, col liveColumn, forcedNullable bool, w *warnings,
) (plannedStatement, error) {
	declaredNullable := f.Optional || forcedNullable
	if declaredNullable == col.Nullable {
		return plannedStatement{}, nil
	}

	switch dialect {
	case atlas.DialectPostgres:
		return renderAlterNullability(dialect, e, f, declaredNullable)
	case atlas.DialectMySQL:
		return renderModifyColumn(dialect, e, f, declaredNullable, kindAlterNullability, mysqlColumnDefinition(col))
	default:
		w.add(
			"nullability change detected on %s.%s (live nullable=%v, declared nullable=%v) but %s has no "+
				"ALTER COLUMN NOT NULL support without a table rebuild; skipping",
			atlas.TableName(e), f.Name, col.Nullable, declaredNullable, dialect)

		return plannedStatement{}, nil
	}
}

// renderAlterNullability renders the ALTER COLUMN statement for a detected
// nullability change. Postgres only; callers must dialect-gate before
// calling this, as nullabilityStatementFor does.
//
// priorType records the state BEFORE this statement runs ("NULL" if the
// column was nullable and this SETs NOT NULL, "NOT NULL" if it was NOT NULL
// and this DROPs it), so a rollback can restore it -- see rollback.go for why
// restoring NOT NULL can fail if a NULL was written in between.
func renderAlterNullability(dialect string, e *ir.Entity, f *ir.Field, declaredNullable bool) (plannedStatement, error) {
	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	col := atlas.QuoteIdent(f.Name)

	action, priorType := "SET NOT NULL", "NULL"
	if declaredNullable {
		action, priorType = "DROP NOT NULL", "NOT NULL"
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s;", table, col, action)

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindAlterNullability, Table: table, Column: f.Name, Statement: sql, PriorType: priorType,
		},
	}, nil
}
