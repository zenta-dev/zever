// Package migrate is zever's live-schema-diff migration engine: it compares
// a compiled .zen schema's resolved *ir.Schema (the same IR the atlas
// backend renders bootstrap DDL from, and the same IR any future zenorm
// codegen backend consumes) against a live database's introspected shape,
// computes the DDL statements that would bring the two in line, and --
// separately -- executes a previously-computed plan while recording a
// checksum of every statement it applies so a re-run is a no-op.
//
// This is adapted, not rebuilt, from orm/migrate's original
// apply/diff/constraints/typeaffinity/
// rollback implementation: the diffing rules, statement rendering,
// and rollback-inversion logic are unchanged, only moved into this
// importable package and split into an explicit compute-a-plan (Plan) /
// execute-a-plan (Apply) pair. cmd/zever's `zever db migrate` and
// `zever db rollback` commands are now thin wrappers over this package's
// public API: flag parsing, prompts, and user-facing output formatting stay
// in cmd/zever; the actual diff/apply/rollback computation lives here.
//
// # Scope
//
// Plan/Apply diff and apply against exactly three dialects: "postgres",
// "sqlite", and "mysql" (the atlas backend's own
// DialectPostgres/DialectSQLite/DialectMySQL constant values, which this
// package intentionally does not redeclare -- a caller already has them from
// internal/dsl/backend/atlas, the same package this package depends on for
// DDL rendering and identifier quoting).
//
// Any other dialect is refused up front: Plan and ComputeRollback both
// return ErrUnsupportedDialect for a db.DB whose Dialect() is none of the
// three, rather than silently no-op'ing or computing a plan that happens to
// be empty.
//
// Within these dialects, several diffing categories are detected but
// deliberately not applied on sqlite specifically, because SQLite has no SQL
// statement that could express the change without a full copy-table rebuild
// (out of scope here): column type changes, column nullability changes, and
// adding a foreign key constraint to an already-existing table. Each such
// case is surfaced as a MigrationPlan.Warnings entry rather than silently
// skipped -- see diff.go, constraints.go, and typeaffinity.go's doc comments
// for the exact rules per category.
//
// MySQL is supported with the same live-diff machinery, using MySQL's own
// spellings: backtick identifiers, MODIFY COLUMN for type/nullability
// changes, DROP INDEX ... ON <table>, and a schema_migrations bookkeeping
// table rendered with MySQL types (see mysql.go). The MySQL gaps that remain
// are the engine's universal ones -- enum value-list changes are not
// detected (the type normalizer reduces ENUM(...) to its base name), column
// defaults are not managed, and a MySQL MODIFY COLUMN therefore does not
// restate an existing default -- all documented where they are decided.
//
// # FTS5
//
// SQLite FTS5 virtual-table CREATE/DROP statement generation is available
// as standalone helpers (CreateFTS5VirtualTable, DropFTS5VirtualTable, in
// fts.go) but is NOT wired into Plan's automatic diff: the .zen DSL carries
// no schema-level signal today marking a field or entity as
// full-text-search-enabled for Plan to key a diff off of. See fts.go for
// the full explanation of that gap and why closing it (a DSL grammar/parser/
// resolver change) is out of scope for this package.
package migrate
