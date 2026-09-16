package atlas

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// TableName returns the SQL table name for an entity: the snake_case form
// of the entity name, pluralized via naming.PluralizeNaive.
//
// The pluralization is deliberately naive and is wrong for irregular
// English nouns ("Person" becomes "persons", "Child" becomes "childs").
// This is a known, documented limitation, decided once in
// internal/dsl/naming rather than reinvented per backend.
func TableName(e *ir.Entity) string {
	return naming.PluralizeNaive(naming.SnakeCase(e.Name))
}

// primaryKeyField returns the entity's @primary field, or nil when the
// entity declares none.
func primaryKeyField(e *ir.Entity) *ir.Field {
	for _, f := range e.Fields {
		if f.Primary {
			return f
		}
	}

	return nil
}

// maxLen reports the @validate(max_len: N) bound declared on a field, if
// any. Only positive bounds count; a non-positive or non-integer bound is
// treated as absent so the column falls back to an unbounded text type.
func maxLen(f *ir.Field) (int64, bool) {
	for _, v := range f.Validate {
		if v.Kind != "max_len" {
			continue
		}

		n, ok := v.Args["value"].(int64)
		if ok && n > 0 {
			return n, true
		}
	}

	return 0, false
}

// foreignKey is one resolved foreign-key constraint, already attributed to
// the table that physically holds the referencing column.
type foreignKey struct {
	Name      string
	Table     string // table holding Column
	Column    string
	RefTable  string
	RefSchema string // Postgres schema of RefTable
	RefColumn string
	OnDelete  string // "" | restrict | cascade | set_null
}

// onDeleteSQL maps the DSL's @on_delete value to the SQL referential
// action keyword used in a DDL REFERENCES clause. An empty or
// unrecognized value yields "", meaning "emit no ON DELETE clause and let
// the database default (NO ACTION) apply".
func onDeleteSQL(v string) string {
	switch v {
	case "restrict":
		return "RESTRICT"
	case "cascade":
		return "CASCADE"
	case "set_null":
		return "SET NULL"
	default:
		return ""
	}
}

// onDeleteHCL maps the DSL's @on_delete value to Atlas's uppercase,
// underscore-separated referential-action identifier (RESTRICT, CASCADE,
// SET_NULL, NO_ACTION). An empty or unrecognized value yields "", meaning
// "emit no on_delete attribute".
func onDeleteHCL(v string) string {
	action := onDeleteSQL(v)
	if action == "" {
		return ""
	}

	return strings.ReplaceAll(action, " ", "_")
}

// collectForeignKeys walks every relation in a module and returns the
// foreign-key constraints it implies, keyed by the table that holds the
// referencing column.
//
// Only belongs_to and has_one produce a constraint: has_many is the
// reciprocal view of a belongs_to (the owning side already emits it) and
// many_to_many needs a join table, which this first cut does not
// synthesize. For belongs_to the foreign key lives on the declaring
// entity; for has_one the resolver places it on the target entity, so the
// constraint is attributed there instead. Constraints are de-duplicated by
// name, so a belongs_to/has_one pair describing the same column collapses
// into one.
func collectForeignKeys(m *ir.Module) map[string][]foreignKey {
	out := make(map[string][]foreignKey)
	seen := make(map[string]bool)

	for _, e := range m.Entities {
		for _, r := range e.Relations {
			fk, ok := relationForeignKey(e, r)
			if !ok || seen[fk.Name] {
				continue
			}

			seen[fk.Name] = true
			out[fk.Table] = append(out[fk.Table], fk)
		}
	}

	return out
}

// relationForeignKey turns one relation into a foreignKey, reporting false
// when the relation implies no constraint (unresolved target or foreign
// key, or a has_many/many_to_many kind).
func relationForeignKey(e *ir.Entity, r *ir.Relation) (foreignKey, bool) {
	if r.Target == nil || r.ForeignKey == nil {
		return foreignKey{}, false
	}

	var holder, ref *ir.Entity

	switch r.Kind {
	case ir.BelongsTo:
		holder, ref = e, r.Target
	case ir.HasOne:
		holder, ref = r.Target, e
	case ir.HasMany, ir.ManyToMany:
		return foreignKey{}, false
	default:
		return foreignKey{}, false
	}

	refPK := primaryKeyField(ref)
	if refPK == nil {
		return foreignKey{}, false
	}

	table := TableName(holder)

	return foreignKey{
		Name:      fmt.Sprintf("%s_%s_fkey", table, r.ForeignKey.Name),
		Table:     table,
		Column:    r.ForeignKey.Name,
		RefTable:  TableName(ref),
		RefSchema: schemaOf(ref),
		RefColumn: refPK.Name,
		OnDelete:  r.OnDelete,
	}, true
}

// schemaOf returns an entity's Postgres schema, defaulting to "public".
// The resolver always populates Entity.Schema, so the fallback is
// defensive only (hand-built IR in tests may leave it empty).
func schemaOf(e *ir.Entity) string {
	if e.Schema == "" {
		return defaultSchema
	}

	return e.Schema
}

// SchemaOf exports schemaOf for callers outside this package (the
// `zengo db migrate` live-introspection path) that need an entity's
// Postgres schema without duplicating the "public" default here.
func SchemaOf(e *ir.Entity) string {
	return schemaOf(e)
}

// QuoteIdent exports quoteIdent for callers outside this package that build
// additional DDL (e.g. ALTER TABLE statements) referencing the same
// identifiers this package quotes for CREATE TABLE/INDEX.
func QuoteIdent(name string) string {
	return quoteIdent(name)
}

// ColumnType returns the SQL column type renderCreateTable would use for a
// field under the given dialect, so callers building other DDL (such as an
// ALTER TABLE ADD COLUMN statement) can reuse the exact same per-dialect
// type mapping instead of duplicating it.
func ColumnType(dialect string, f *ir.Field) (string, error) {
	if err := checkDialect(dialect); err != nil {
		return "", err
	}

	return sqlColumnType(dialect, f), nil
}

// QualifiedTableName returns an entity's table name, schema-qualified for
// Postgres exactly as CREATE TABLE statements are, for callers building
// other DDL against the same table.
func QualifiedTableName(dialect string, e *ir.Entity) (string, error) {
	if err := checkDialect(dialect); err != nil {
		return "", err
	}

	return qualifiedTable(dialect, e), nil
}

// nullableColumns returns the set of column names on a table that must be
// nullable due to a foreign key with @on_delete(set_null), which the
// database cannot honor on a NOT NULL column. Fields marked optional
// (`field: type?`) are made nullable independently by the callers below via
// ir.Field.Optional; this only covers the foreign-key-forced case.
func nullableColumns(fks []foreignKey) map[string]bool {
	out := make(map[string]bool, len(fks))

	for _, fk := range fks {
		if fk.OnDelete == "set_null" {
			out[fk.Column] = true
		}
	}

	return out
}

// entityIndexes returns the indexes to emit for an entity: one unique
// index per @unique non-primary field, followed by every declared
// index(...) block. The @primary field is skipped because the primary key
// constraint already implies a unique index on it.
func entityIndexes(e *ir.Entity) []indexSpec {
	table := TableName(e)

	var out []indexSpec

	for _, f := range e.Fields {
		if !f.Unique || f.Primary {
			continue
		}

		out = append(out, indexSpec{
			Name:    fmt.Sprintf("%s_%s_key", table, f.Name),
			Columns: []string{f.Name},
			Unique:  true,
		})
	}

	return append(out, declaredIndexes(e)...)
}

// declaredIndexes returns a spec for each explicit index(...) block on an
// entity, skipping any block whose column list failed to resolve.
func declaredIndexes(e *ir.Entity) []indexSpec {
	table := TableName(e)

	var out []indexSpec

	for _, idx := range e.Indexes {
		spec := indexSpec{Unique: idx.Unique}

		for _, c := range idx.Columns {
			if c != nil {
				spec.Columns = append(spec.Columns, c.Name)
			}
		}

		if len(spec.Columns) == 0 {
			continue
		}

		suffix := "idx"
		if idx.Unique {
			suffix = "key"
		}

		spec.Name = fmt.Sprintf("%s_%s_%s", table, strings.Join(spec.Columns, "_"), suffix)

		out = append(out, spec)
	}

	return out
}

// indexSpec is one index to emit on a table.
type indexSpec struct {
	Name    string
	Columns []string
	Unique  bool
}

// IndexSpec exports indexSpec for callers outside this package (the
// `zengo db migrate` live-diffing path) that need to compare the DECLARED
// schema side of an index against live database state, using the exact same
// names the bootstrap DDL renderer already uses -- so a table created by
// RenderSchemaDDL diffs as already-in-sync against itself.
type IndexSpec = indexSpec

// ForeignKey exports foreignKey for the same reason IndexSpec exports
// indexSpec: the same names and derivation rules the bootstrap DDL renderer
// uses, reused by the live-diffing path instead of duplicated.
type ForeignKey = foreignKey

// DeclaredIndexes exports declaredIndexes: the entity's explicit
// index(...) blocks only, NOT the unique indexes implied by @unique fields
// (renderColumnDDL emits those as inline column constraints, and the live
// migration diff treats them separately -- see EntityForeignKeys/
// NullableColumns for the equivalent foreign-key-side exports).
func DeclaredIndexes(e *ir.Entity) []IndexSpec {
	return declaredIndexes(e)
}

// EntityForeignKeys returns the foreign-key constraints an entity's own
// table holds, per the same belongs_to/has_one derivation collectForeignKeys
// uses for bootstrap rendering. Requires e.Module to be set (always true for
// resolver-produced IR; nil only for hand-built IR in tests).
func EntityForeignKeys(e *ir.Entity) []ForeignKey {
	if e == nil || e.Module == nil {
		return nil
	}

	return collectForeignKeys(e.Module)[TableName(e)]
}

// NullableColumns exports nullableColumns for callers outside this package
// needing the same "which columns must be nullable due to an
// @on_delete(set_null) foreign key" rule the bootstrap renderer applies, so
// nullability diffing does not flag a column as changed when the live
// database is nullable only because of that forced case.
func NullableColumns(fks []ForeignKey) map[string]bool {
	return nullableColumns(fks)
}
