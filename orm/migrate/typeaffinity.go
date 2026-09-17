package migrate

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// This file holds the type-comparison half of the diffing engine: turning a
// dialect-native type spelling (either the one atlas renders from the
// schema, or the one introspected from a live database) into a form the two
// can be compared in. Moved from orm/migrate/typeaffinity.go.
//
// The guiding rule throughout is: PREFER FALSE NEGATIVES. A type change this
// code fails to notice leaves the database as it was, which is recoverable.
// A type change this code hallucinates fires a destructive ALTER COLUMN TYPE
// on every single run, which is not.

// normalizeType collapses a dialect-native type spelling into a canonical
// form comparable against atlas.ColumnType's own output for the same scalar
// type, normalized the same way. It is not a full type-system unifier: it
// only needs to answer "does this live type still match what the schema
// declares".
//
// Postgres: length/precision is dropped (information_schema.data_type never
// carries it anyway), the name is lowercased, and known spellings are folded
// onto one representative -- "character varying" and "varchar" both become
// "varchar", "timestamp with time zone" and atlas's own "timestamptz" both
// become "timestamp".
//
// SQLite: the declared type is reduced to its storage AFFINITY, never
// compared as a string. See sqliteAffinity.
//
// MySQL: length/precision is dropped, the name is lowercased, known spellings
// are folded onto one representative, and modifiers the DSL cannot express
// (UNSIGNED, ZEROFILL, AUTO_INCREMENT) are ignored. See mysqlNormalize.
func normalizeType(dialect, sqlType string) string {
	base := strings.ToLower(strings.TrimSpace(stripTypeArgs(sqlType)))
	base = strings.Join(strings.Fields(base), " ")

	switch dialect {
	case atlas.DialectSQLite:
		return sqliteAffinity(sqlType)
	case atlas.DialectPostgres:
		if canonical, ok := postgresTypeSynonyms[base]; ok {
			return canonical
		}

		return base
	case atlas.DialectMySQL:
		return mysqlNormalize(base)
	default:
		return base
	}
}

// postgresTypeSynonyms folds the spellings Postgres accepts, reports through
// information_schema.data_type, and that atlas.ColumnType emits, onto one
// representative each. Anything absent here normalizes to its own lowercased,
// argument-stripped name.
var postgresTypeSynonyms = map[string]string{
	"character varying":           "varchar",
	"varchar":                     "varchar",
	"character":                   "char",
	"char":                        "char",
	"bpchar":                      "char",
	"boolean":                     "bool",
	"bool":                        "bool",
	"timestamp with time zone":    "timestamp",
	"timestamp without time zone": "timestamp",
	"timestamptz":                 "timestamp",
	"timestamp":                   "timestamp",
	"time with time zone":         "time",
	"time without time zone":      "time",
	"timetz":                      "time",
	"time":                        "time",
	"integer":                     "integer",
	"int":                         "integer",
	"int4":                        "integer",
	"bigint":                      "bigint",
	"int8":                        "bigint",
	"smallint":                    "smallint",
	"int2":                        "smallint",
	"real":                        "real",
	"float4":                      "real",
	"double precision":            "double precision",
	"float8":                      "double precision",
	"numeric":                     "numeric",
	"decimal":                     "numeric",
}

// stripTypeArgs removes a parenthesized length/precision suffix, so
// "varchar(255)" and "character varying" reduce to the same base name.
func stripTypeArgs(sqlType string) string {
	if i := strings.IndexByte(sqlType, '('); i >= 0 {
		return sqlType[:i]
	}

	return sqlType
}

// mysqlTypeSynonyms folds the spellings MySQL accepts, reports through
// information_schema.COLUMN_TYPE, and that atlas.ColumnType emits, onto one
// representative each. Anything absent here normalizes to its own lowercased,
// argument-stripped name.
//
// BOOLEAN/BOOL fold onto TINYINT because MySQL implements BOOLEAN as a
// TINYINT(1) alias and reports the column as tinyint. DOUBLE PRECISION folds
// onto DOUBLE for the same reason. ENUM keeps its value list stripped by
// stripTypeArgs, so an enum value-set change is deliberately NOT detected
// (see mysqlNormalize's doc comment).
var mysqlTypeSynonyms = map[string]string{
	"integer":           "int",
	"int":               "int",
	"mediumint":         "mediumint",
	"smallint":          "smallint",
	"tinyint":           "tinyint",
	"bool":              "tinyint",
	"boolean":           "tinyint",
	"bigint":            "bigint",
	"decimal":           "decimal",
	"dec":               "decimal",
	"numeric":           "decimal",
	"fixed":             "decimal",
	"float":             "float",
	"double":            "double",
	"double precision":  "double",
	"real":              "double",
	"character varying": "varchar",
	"varchar":           "varchar",
	"character":         "char",
	"char":              "char",
	"nvarchar":          "varchar",
	"text":              "text",
	"tinytext":          "tinytext",
	"mediumtext":        "mediumtext",
	"longtext":          "longtext",
	"blob":              "blob",
	"tinyblob":          "tinyblob",
	"mediumblob":        "mediumblob",
	"longblob":          "longblob",
	"binary":            "binary",
	"varbinary":         "varbinary",
	"datetime":          "datetime",
	"timestamp":         "timestamp",
	"date":              "date",
	"time":              "time",
	"year":              "year",
	"json":              "json",
	"enum":              "enum",
	"set":               "set",
}

// mysqlNormalize canonicalizes an already-lowercased, argument-stripped MySQL
// type name. Modifiers the DSL cannot express are removed rather than
// compared: a live column that is UNSIGNED or AUTO_INCREMENT while the schema
// declares the plain type is NOT reported as a change, because the schema has
// no way to say otherwise and inventing a MODIFY for it would silently drop
// the modifier on every run. This is the same "prefer false negatives"
// posture the rest of this file takes.
func mysqlNormalize(base string) string {
	for _, modifier := range []string{" unsigned", " zerofill", " auto_increment"} {
		base = strings.ReplaceAll(base, modifier, "")
	}

	base = strings.Join(strings.Fields(base), " ")

	if canonical, ok := mysqlTypeSynonyms[base]; ok {
		return canonical
	}

	return base
}

// SQLite storage affinity classes, per
// https://sqlite.org/datatype3.html#determination_of_column_affinity.
const (
	affinityInteger = "INTEGER"
	affinityText    = "TEXT"
	affinityBlob    = "BLOB"
	affinityReal    = "REAL"
	affinityNumeric = "NUMERIC"
)

// sqliteAffinity implements SQLite's real column-affinity algorithm, in its
// documented rule order:
//
//  1. contains "INT"                     -> INTEGER
//  2. contains "CHAR", "CLOB" or "TEXT"  -> TEXT
//  3. contains "BLOB", or is empty       -> BLOB
//  4. contains "REAL", "FLOA" or "DOUB"  -> REAL (prefixes of DOUBLE, FLOAT)
//  5. otherwise                          -> NUMERIC
//
// SQLite's typing is dynamic: a column declared BIGINT, INT or SMALLINT is
// the same column as far as the engine is concerned. Comparing declared type
// STRINGS on sqlite would therefore report a type change on every run for
// every schema whose spelling differs from what the table was created with.
// Affinity classes are the only comparison that means anything here.
//
//nolint:misspell // "DOUB" is the DOUBLE prefix per SQLite affinity rules
func sqliteAffinity(declaredType string) string {
	t := strings.ToUpper(strings.TrimSpace(declaredType))

	switch {
	case strings.Contains(t, "INT"):
		return affinityInteger
	case strings.Contains(t, "CHAR"), strings.Contains(t, "CLOB"), strings.Contains(t, "TEXT"):
		return affinityText
	case t == "", strings.Contains(t, "BLOB"):
		return affinityBlob
	case strings.Contains(t, "REAL"), strings.Contains(t, "FLOA"), strings.Contains(t, "DOUB"): //nolint:misspell // "DOUB" is the DOUBLE prefix per SQLite affinity rules
		return affinityReal
	default:
		return affinityNumeric
	}
}

// typeChanged reports whether a field's declared type and a live column's
// introspected type differ once normalized for the dialect.
//
// A live column with no captured type at all reports false: an unknown type
// is not evidence of a change, and inventing one here would fire a spurious
// ALTER on every run. Dialects without a normalization rule likewise always
// report false.
func typeChanged(dialect string, f *ir.Field, live liveColumn) (bool, error) {
	switch dialect {
	case atlas.DialectPostgres, atlas.DialectSQLite, atlas.DialectMySQL:
	default:
		return false, nil
	}

	if strings.TrimSpace(live.RawType) == "" {
		return false, nil
	}

	declared, err := atlas.ColumnType(dialect, f)
	if err != nil {
		return false, err
	}

	return normalizeType(dialect, declared) != normalizeType(dialect, live.RawType), nil
}

// renderAlterColumnType emits the DDL for a detected type change.
//
// Postgres only. SQLite has NO ALTER COLUMN TYPE statement of any kind -- the
// only way to change a column's declared type there is the twelve-step
// copy-to-a-new-table rebuild, a materially different and much riskier code
// path that is explicitly out of scope. Callers on sqlite must warn and skip
// rather than ask this function for a statement; it returns an error if they
// don't, instead of emitting SQL sqlite would reject.
//
// The USING clause is always emitted, even for widening casts: Postgres
// refuses any non-trivially-castable change without one, and an explicit
// cast makes the intent of a narrowing change visible in the DDL rather than
// leaving it to fail at execution time.
//
// priorType is the live column's type as introspected before this statement
// runs. It is recorded so a rollback can put the column back the way it was;
// note that a narrowing change (bigint -> smallint, text -> varchar(8)) is
// still LOSSY in both directions -- the reverse ALTER restores the type but
// not truncated values. That is a limitation of DDL-level rollback in general,
// not something this implementation could fix.
func renderAlterColumnType(dialect string, e *ir.Entity, f *ir.Field, priorType string) (plannedStatement, error) {
	if dialect != atlas.DialectPostgres {
		return plannedStatement{}, fmt.Errorf("[orm/migrate] dialect %q has no ALTER COLUMN TYPE support", dialect)
	}

	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	colType, err := atlas.ColumnType(dialect, f)
	if err != nil {
		return plannedStatement{}, err
	}

	col := atlas.QuoteIdent(f.Name)

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;", table, col, colType, col, colType)

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kindAlterType, Table: table, Column: f.Name, Statement: sql, PriorType: priorType,
		},
	}, nil
}

// mysqlColumnDefinition renders a live column's full definition -- its raw
// type plus its nullability -- in the shape a MySQL MODIFY COLUMN inverse
// needs. It is the prior-state counterpart to the declared column definition
// renderModifyColumn emits forward: MySQL has no "ALTER COLUMN TYPE" or
// "ALTER COLUMN SET/DROP NOT NULL", so both the forward change and its
// rollback must restate the whole column.
//
// Defaults are deliberately not captured: this engine does not manage column
// defaults on any dialect (see liveColumn.HasDefault, which is observed but
// never diffed), and restating an unknown default would risk inventing one.
func mysqlColumnDefinition(col liveColumn) string {
	def := strings.TrimSpace(col.RawType)
	if !col.Nullable {
		def += " NOT NULL"
	}

	return def
}

// renderModifyColumn emits the MySQL ALTER TABLE ... MODIFY COLUMN statement
// for a detected type and/or nullability change. MySQL only, because MODIFY
// COLUMN is MySQL's syntax; Postgres uses renderAlterColumnType /
// renderAlterNullability and SQLite has neither (it warns and skips, like
// typeChangeStatementFor documents).
//
// MODIFY COLUMN restates the WHOLE column definition, and a MySQL MODIFY
// that omits a clause silently resets it to its default -- so both the
// declared type and the declared nullability are always emitted. Omitting
// NOT NULL would silently make the column nullable; omitting the type would
// make it an error.
//
// priorDef is the live column's full definition (mysqlColumnDefinition)
// captured before the statement runs, so a rollback can put the column back.
func renderModifyColumn(
	dialect string, e *ir.Entity, f *ir.Field, nullable bool, kind, priorDef string,
) (plannedStatement, error) {
	if dialect != atlas.DialectMySQL {
		return plannedStatement{}, fmt.Errorf("[orm/migrate] dialect %q has no MODIFY COLUMN support", dialect)
	}

	table, err := atlas.QualifiedTableName(dialect, e)
	if err != nil {
		return plannedStatement{}, err
	}

	colType, err := atlas.ColumnType(dialect, f)
	if err != nil {
		return plannedStatement{}, err
	}

	nullability := " NOT NULL"
	if nullable {
		nullability = " NULL"
	}

	//lint:allow-unsafesql identifier is schema-introspected, not user input
	sql := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s%s;",
		table, quoteIdent(dialect, f.Name), colType, nullability)

	return plannedStatement{
		SQL: sql,
		Meta: migrationMeta{
			Kind: kind, Table: table, Column: f.Name, Statement: sql, PriorType: priorDef,
		},
	}, nil
}
