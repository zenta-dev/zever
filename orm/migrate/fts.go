package migrate

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

// This file holds standalone SQLite FTS5 virtual-table DDL helpers.
//
// WHY THESE ARE STANDALONE, NOT WIRED INTO Plan's AUTOMATIC DIFF: Plan's diff
// is driven entirely by the resolved *ir.Schema/*ir.Entity/*ir.Field the
// compiler hands it. As of this package's introduction, the .zen DSL's
// grammar, lexer, parser, and resolver (internal/dsl/{lexer,parser,resolver})
// carry no annotation at all marking a field or entity as full-text-search
// enabled -- there is no "@fts" field attribute, no "fts(...)" entity block,
// nothing in ir.Field or ir.Entity a diff pass could key off of. Grepping
// internal/dsl/ir and internal/dsl/parser for "fts"/"fulltext"/"search"
// confirms this: the only "search" in this module is the unrelated
// search/postgres package (tsvector/ts_rank), which this migration engine
// does not touch.
//
// Adding that schema-level signal is a DSL grammar/parser/resolver change --
// new syntax, new IR fields, new resolver validation -- and is explicitly out
// of scope for this phase (see docs-db-orm/PLAN.md, Phase 7). Until a future
// phase adds it, these two functions are the full extent of FTS5 support
// here: a caller that wants a SQLite FTS5 virtual table today constructs the
// statement itself via CreateFTS5VirtualTable/DropFTS5VirtualTable and runs
// it however it likes (e.g. as an extra statement alongside a Plan/Apply
// call) -- Plan will never emit one on its own, and never drops one it did
// not create either, since it has no declared-vs-live signal to compare
// against at all.

// CreateFTS5VirtualTable renders a SQLite
// `CREATE VIRTUAL TABLE ... USING fts5(...)` statement for the given table
// name and column list. Every identifier is validated (see validateIdent)
// before being interpolated, the same defense-in-depth every other DDL
// renderer in this package applies.
//
// Not currently invertible: no "drop_fts5_table" migrationMeta kind exists,
// since nothing in Plan's diff ever creates one automatically to need
// inverting -- a caller driving this directly is responsible for its own
// rollback story.
func CreateFTS5VirtualTable(tableName string, columns []string) (string, error) {
	if err := validateIdent(tableName); err != nil {
		return "", err
	}

	if len(columns) == 0 {
		return "", fmt.Errorf("[orm/migrate] CreateFTS5VirtualTable %q: at least one column is required", tableName)
	}

	quoted := make([]string, len(columns))

	for i, c := range columns {
		if err := validateIdent(c); err != nil {
			return "", err
		}

		quoted[i] = c
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	return fmt.Sprintf("CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(%s);",
		atlas.QuoteIdent(tableName), strings.Join(quoted, ", ")), nil
}

// DropFTS5VirtualTable renders the statement that drops a SQLite FTS5
// virtual table. An FTS5 virtual table is dropped exactly like any other
// table -- DROP TABLE, not a dedicated FTS statement -- but this helper is
// named for symmetry with CreateFTS5VirtualTable and to keep both halves of
// the (still unwired) FTS5 lifecycle discoverable from one place.
func DropFTS5VirtualTable(tableName string) string {
	//lint:allow-unsafesql identifier is schema-introspected, not user input
	return fmt.Sprintf("DROP TABLE IF EXISTS %s;", atlas.QuoteIdent(tableName))
}
