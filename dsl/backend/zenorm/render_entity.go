package zenorm

// This file renders one ir.Entity into its section of its module's Go
// source file: a package-level orm.Table[Entity] var, a package-level
// <Entity>Cols struct of typed orm.Column/orm.NullableColumn vars, the
// plain entity struct itself, and a positional, zero-reflection Scan
// method.
//
// Unlike flat per-entity Field vars, the orm Column/NullableColumn values
// are grouped under one <Entity>Cols struct literal, so this phase emits
// exactly that shape.

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// renderEntityBody appends one entity's whole generated section to b.
func renderEntityBody(b *strings.Builder, m entityModel, schemaName string) {
	renderTableVar(b, m)
	renderColsVar(b, m)
	renderStruct(b, m, schemaName)
	renderScan(b, m)
	renderColumnsMethod(b, m)
	renderRelations(b, m)
}

// renderTableVar emits the package-level orm.Table[Entity] var.
func renderTableVar(b *strings.Builder, m entityModel) {
	cols := make([]string, 0, len(m.Fields))
	for _, f := range m.Fields {
		cols = append(cols, fmt.Sprintf("%q", f.Column))
	}

	fmt.Fprintf(b, "// %s is the orm.Table for %s, generated from the %q entity.\n", m.TableVar, m.Name, m.Name)
	fmt.Fprintf(b, "var %s = orm.NewTable[%s](%q, []string{%s})\n\n", m.TableVar, m.Name, m.Table, strings.Join(cols, ", "))
}

// renderColsVar emits the package-level <Entity>Cols struct of typed
// orm.Column/orm.NullableColumn vars, one per scalar field.
func renderColsVar(b *strings.Builder, m entityModel) {
	fmt.Fprintf(b, "// %s holds typed orm.Column/orm.NullableColumn references for %s,\n", m.ColsVar, m.Name)
	b.WriteString("// used to build type-safe predicates, order terms and assignments.\n")
	fmt.Fprintf(b, "var %s = struct {\n", m.ColsVar)

	for _, f := range m.Fields {
		if f.Optional {
			fmt.Fprintf(b, "\t%s orm.NullableColumn[%s, %s]\n", f.GoName, m.Name, f.GoType)

			continue
		}

		fmt.Fprintf(b, "\t%s orm.Column[%s, %s]\n", f.GoName, m.Name, f.GoType)
	}

	b.WriteString("}{\n")

	for _, f := range m.Fields {
		if f.Optional {
			fmt.Fprintf(b, "\t%s: orm.NewNullableColumn[%s, %s](%q, %q),\n", f.GoName, m.Name, f.GoType, m.Table, f.Column)

			continue
		}

		fmt.Fprintf(b, "\t%s: orm.NewColumn[%s, %s](%q, %q),\n", f.GoName, m.Name, f.GoType, m.Table, f.Column)
	}

	b.WriteString("}\n\n")
}

// renderStruct emits the entity data struct: every scalar column becomes a
// bare GoType field, or an orm.Option[GoType] field when optional -- never
// a "*GoType" pointer field.
func renderStruct(b *strings.Builder, m entityModel, schemaName string) {
	fmt.Fprintf(b, "// %s mirrors the %q entity declared in the schema.\n", m.Name, schemaName)
	fmt.Fprintf(b, "type %s struct {\n", m.Name)

	for _, f := range m.Fields {
		goType := f.GoType
		if f.Optional {
			goType = "orm.Option[" + f.GoType + "]"
		}

		fmt.Fprintf(b, "\t%s %s `json:%q`\n", f.GoName, goType, f.Column)
	}

	b.WriteString("}\n\n")
}

// renderScan emits the entity's positional, zero-reflection Scan method.
// Every field but a non-optional timestamp/date scans directly into its
// struct field's address: a bare scalar (string/int32/int64/float32/
// float64/bool/[]byte) is handled by database/sql's own reflection-free
// numeric/string conversions, and orm.Option[T] implements sql.Scanner
// itself. A non-optional timestamp/date column is the one exception --
// time.Time implements neither sql.Scanner nor one of database/sql's
// built-in numeric/string dest kinds -- so it is scanned into a raw string
// first and parsed via time.Parse(time.RFC3339Nano, ...), matching the
// RFC3339Nano text orm's sqlite path binds. An OPTIONAL
// timestamp/date column needs no such special-casing:
// orm.Option[time.Time] already parses an incoming string/[]byte value
// itself, so it scans directly like every other nullable column.
func renderScan(b *strings.Builder, m entityModel) {
	fmt.Fprintf(b, "// Scan reads one row, whose columns must be in %s.Columns() order, into e.\n", m.TableVar)
	fmt.Fprintf(b, "func (e *%s) Scan(row orm.Row) error {\n", m.Name)

	targets := make([]string, 0, len(m.Fields))

	for _, f := range m.Fields {
		if f.IsTime && !f.Optional {
			fmt.Fprintf(b, "\tvar raw%s string\n", f.GoName)

			targets = append(targets, "&raw"+f.GoName)

			continue
		}

		targets = append(targets, "&e."+f.GoName)
	}

	fmt.Fprintf(b, "\tif err := row.Scan(%s); err != nil {\n", strings.Join(targets, ", "))
	fmt.Fprintf(b, "\t\treturn fmt.Errorf(\"[%s] scan error: %%w\", err)\n\t}\n", m.Pkg)

	for _, f := range m.Fields {
		if !f.IsTime || f.Optional {
			continue
		}

		fmt.Fprintf(b, "\n\tval%s, err%s := time.Parse(time.RFC3339Nano, raw%s)\n", f.GoName, f.GoName, f.GoName)
		fmt.Fprintf(b, "\tif err%s != nil {\n", f.GoName)
		fmt.Fprintf(b, "\t\treturn fmt.Errorf(\"[%s] parse %s error: %%w\", err%s)\n\t}\n", m.Pkg, f.Column, f.GoName)
		fmt.Fprintf(b, "\n\te.%s = val%s\n", f.GoName, f.GoName)
	}

	b.WriteString("\n\treturn nil\n}\n\n")
}

// renderColumnsMethod emits Columns(), the field-declaration-order column
// list Scan reads by -- exposed as a value method (not tied to a pointer
// receiver like Scan) so a zero-value Entity{} can report it too.
func renderColumnsMethod(b *strings.Builder, m entityModel) {
	cols := make([]string, 0, len(m.Fields))
	for _, f := range m.Fields {
		cols = append(cols, fmt.Sprintf("%q", f.Column))
	}

	fmt.Fprintf(b, "// Columns lists %s's columns in field-declaration order; Scan reads a\n", m.Name)
	b.WriteString("// row's columns in exactly this order.\n")
	fmt.Fprintf(b, "func (%s) Columns() []string { return []string{%s} }\n\n", m.Name, strings.Join(cols, ", "))
}

// pascalModuleLabel is used by render_module.go for the package doc
// comment.
func pascalModuleLabel(name string) string {
	if name == "" {
		return "application"
	}

	return naming.PascalCase(name)
}
