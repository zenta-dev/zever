package resolver

import (
	"sort"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// scalarTypes maps the fixed set of v1 scalar type names to their ir
// constant. "enum" is deliberately absent: it's handled separately by
// resolveFieldType because, unlike every other type name, it takes an arg
// list.
var scalarTypes = map[string]ir.ScalarType{
	"uuid": ir.TUUID, "string": ir.TString, "int32": ir.TInt32, "int64": ir.TInt64,
	"float32": ir.TFloat32, "float64": ir.TFloat64, "bool": ir.TBool,
	"timestamp": ir.TTimestamp, "date": ir.TDate, "bytes": ir.TBytes, "json": ir.TJSON,
}

// ScalarTypeNames returns the canonical, sorted list of builtin scalar type
// names recognized by resolveFieldType (the keys of scalarTypes). "enum" is
// deliberately excluded: it resolves through a separate arg-list path
// rather than the scalarTypes table.
//
// This is the single source of truth for the DSL's scalar type names; it is
// exported so codegen tooling (see internal/dsl/gengrammar) can derive
// editor grammars from it instead of duplicating the list.
func ScalarTypeNames() []string {
	names := make([]string, 0, len(scalarTypes))
	for name := range scalarTypes {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// resolveEntities is Pass 1's entry point: it resolves every entity decl
// (in Pass 0's source-declaration order) independently of every other
// entity, and appends each resolved *ir.Entity to its owning Module's
// Entities slice — the reverse direction of Entity.Module, and the thing
// that makes ir.Module.Entities non-empty.
func resolveEntities(decls []*ast.EntityDecl, declModule map[ast.Decl]*ir.Module, enumByName map[string]*ir.Enum) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants (every accepted
			// entity decl has an owning module); guarded defensively so a
			// future bug here reports nothing rather than panics.
			continue
		}

		entity, d := resolveEntity(decl, module, enumByName)
		diags = append(diags, d...)

		module.Entities = append(module.Entities, entity)
	}

	return diags
}

// resolveEntity resolves one entity's fields and entity-level attributes.
func resolveEntity(decl *ast.EntityDecl, module *ir.Module, enumByName map[string]*ir.Enum) (*ir.Entity, diag.List) {
	var diags diag.List

	fields := make([]*ir.Field, 0, len(decl.Fields))

	var primaryPos []diag.Position

	for _, fd := range decl.Fields {
		field, d := resolveField(decl, fd, enumByName)
		diags = append(diags, d...)

		if field == nil {
			continue
		}

		if field.Primary {
			primaryPos = append(primaryPos, fd.Pos)
		}

		fields = append(fields, field)
	}

	diags = append(diags, checkPrimaryCardinality(decl, primaryPos)...)

	schemaName, d := resolveEntitySchema(decl, module)
	diags = append(diags, d...)

	entity := &ir.Entity{
		Name:       decl.Name,
		Module:     module,
		Schema:     schemaName,
		Fields:     fields,
		Pos:        decl.Pos,
		DocComment: decl.DocComment,
	}

	return entity, diags
}

// checkPrimaryCardinality enforces exactly one @primary field: zero is an
// error, two-or-more is an error naming both the first and the offending
// second position.
func checkPrimaryCardinality(decl *ast.EntityDecl, primaryPos []diag.Position) diag.List {
	switch len(primaryPos) {
	case 0:
		return diag.List{diag.Wrap("resolve", decl.Pos, ErrInvalidAttribute,
			"entity %q must declare exactly one @primary field, found none", decl.Name)}
	case 1:
		return nil
	default:
		return diag.List{diag.Wrap("resolve", primaryPos[1], ErrInvalidAttribute,
			"entity %q declares multiple @primary fields (first at %s)", decl.Name, primaryPos[0].String())}
	}
}

// resolveField resolves one field's type and attributes. It returns a nil
// *ir.Field (and no further attribute diagnostics) when the type itself
// fails to resolve, since there is nothing meaningful left to attach
// attributes to.
//
// decl is the owning entity, needed only by @renamed_from, whose validity
// depends on the other field names declared alongside this one.
func resolveField(decl *ast.EntityDecl, fd *ast.FieldDecl, enumByName map[string]*ir.Enum) (*ir.Field, diag.List) {
	ft, ftDiag := resolveFieldType(fd.Type, enumByName)
	if ftDiag != nil {
		return nil, diag.List{ftDiag}
	}

	var diags diag.List

	field := &ir.Field{Name: fd.Name, Type: ft, Optional: fd.Optional, Pos: fd.Pos, DocComment: fd.DocComment}

	for _, attr := range fd.Attributes {
		diags = append(diags, resolveFieldAttribute(decl, fd, attr, field)...)
	}

	return field, diags
}

// resolveFieldType resolves a TypeExpr's static Name/Args into an
// ir.FieldType. It is shared, unmodified, with RPC/job param resolution.
//
// "enum" requires at least one arg and no duplicate value (anonymous enum).
// A bare identifier matching a known named enum resolves to that enum
// (EnumName set, no args allowed). A known scalar name must carry no arg
// list. Anything else is an unresolved reference — including a bare relation
// keyword typo'd into a field position, hence the pointer to how relations
// are actually declared.
func resolveFieldType(te *ast.TypeExpr, enumByName map[string]*ir.Enum) (ir.FieldType, *diag.Diagnostic) {
	if te == nil {
		return ir.FieldType{}, diag.Wrap("resolve", diag.Position{}, ErrUnresolvedReference,
			"field/param declares no type expression")
	}

	if te.Name == "enum" {
		return resolveEnumType(te)
	}

	if enum, ok := enumByName[te.Name]; ok {
		if len(te.Args) > 0 {
			return ir.FieldType{}, diag.Wrap("resolve", te.Pos, ErrInvalidAttribute,
				"enum %q does not take arguments", te.Name)
		}

		return ir.FieldType{Scalar: ir.TEnum, EnumValues: enum.Values, EnumName: enum.Name, Pos: te.Pos}, nil
	}

	scalar, ok := scalarTypes[te.Name]
	if !ok {
		return ir.FieldType{}, diag.Wrap("resolve", te.NamePos, ErrUnresolvedReference,
			"unknown field type %q (declare relations with has_many/has_one/belongs_to/many_to_many, not as a field)", te.Name)
	}

	if len(te.Args) > 0 {
		return ir.FieldType{}, diag.Wrap("resolve", te.Pos, ErrInvalidAttribute,
			"type %q does not take arguments", te.Name)
	}

	return ir.FieldType{Scalar: scalar, Pos: te.Pos}, nil
}

// resolveEnumType resolves the "enum(a, b, c)" TypeExpr form.
func resolveEnumType(te *ast.TypeExpr) (ir.FieldType, *diag.Diagnostic) {
	if len(te.Args) == 0 {
		return ir.FieldType{}, diag.Wrap("resolve", te.Pos, ErrInvalidAttribute,
			"enum type requires at least one value")
	}

	seen := make(map[string]bool, len(te.Args))

	for i, v := range te.Args {
		if seen[v] {
			pos := te.Pos
			if i < len(te.ArgPos) {
				pos = te.ArgPos[i]
			}

			return ir.FieldType{}, diag.Wrap("resolve", pos, ErrInvalidAttribute, "duplicate enum value %q", v)
		}

		seen[v] = true
	}

	return ir.FieldType{Scalar: ir.TEnum, EnumValues: te.Args, Pos: te.Pos}, nil
}

// resolveFieldAttribute dispatches one field-level Attribute to the
// matching handler. Any name besides
// primary/unique/validate/default/renamed_from is ErrInvalidAttribute.
func resolveFieldAttribute(decl *ast.EntityDecl, fd *ast.FieldDecl, attr *ast.Attribute, field *ir.Field) diag.List {
	switch attr.Name {
	case "primary":
		field.Primary = true
		return nil
	case "unique":
		field.Unique = true
		return nil
	case "validate":
		vals, diags := resolveValidateRules(attr, field.Type.Scalar)
		field.Validate = append(field.Validate, vals...)

		return diags
	case "default":
		return resolveDefaultAttribute(attr, field)
	case "renamed_from":
		old, diags := resolveRenamedFromAttribute(decl, fd, attr)
		if old != nil {
			field.RenamedFrom = old
		}

		return diags
	default:
		return diag.List{diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "unknown field attribute @%s", attr.Name)}
	}
}

// resolveRenamedFromAttribute decodes @renamed_from("old_name"), which
// records that this field's column used to be called something else so that
// `zengo db migrate` can emit a RENAME COLUMN instead of the drop+add pair a
// names-only diff would otherwise produce.
//
// It requires exactly one non-empty string-literal argument naming a column
// that is not itself a currently-declared field on the same entity: renaming
// a column onto a name the entity still declares (or onto its own name) is
// nonsensical, and would make the migration plan self-contradictory.
func resolveRenamedFromAttribute(
	entity *ast.EntityDecl, field *ast.FieldDecl, attr *ast.Attribute,
) (*string, diag.List) {
	if len(attr.Args) != 1 {
		return nil, diag.List{diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
			"@renamed_from requires exactly one argument")}
	}

	lit, ok := attr.Args[0].Value.(*ast.StringLit)
	if !ok {
		return nil, diag.List{diag.Wrap("resolve", attr.Args[0].Pos, ErrInvalidAttribute,
			"@renamed_from argument must be a string")}
	}

	if lit.Value == "" {
		return nil, diag.List{diag.Wrap("resolve", lit.Pos, ErrInvalidAttribute,
			"@renamed_from old column name must not be empty")}
	}

	if lit.Value == field.Name {
		return nil, diag.List{diag.Wrap("resolve", lit.Pos, ErrInvalidAttribute,
			"@renamed_from names field %q itself, which is not a rename", field.Name)}
	}

	for _, other := range entity.Fields {
		if other == field {
			continue
		}

		if other.Name == lit.Value {
			return nil, diag.List{diag.Wrap("resolve", lit.Pos, ErrInvalidAttribute,
				"@renamed_from old column %q is still declared as a field on entity %q", lit.Value, entity.Name)}
		}
	}

	old := lit.Value

	return &old, nil
}

// resolveDefaultAttribute resolves one @default(...) attribute. now() is
// only valid on TTimestamp fields; a bare identifier default is either
// "true"/"false" on a TBool field, or on TEnum fields must name a declared
// enum value (else ErrUnresolvedReference); a literal default's decoded
// kind must match the field's scalar family.
func resolveDefaultAttribute(attr *ast.Attribute, field *ir.Field) diag.List {
	if len(attr.Args) != 1 {
		return diag.List{diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "@default requires exactly one argument")}
	}

	switch v := attr.Args[0].Value.(type) {
	case *ast.CallValue:
		return resolveDefaultCall(v, field)
	case *ast.IdentValue:
		return resolveDefaultIdent(v, field)
	case *ast.StringLit:
		return resolveDefaultLiteral(v.Pos, v.Value, field, ir.TString, ir.TUUID, ir.TDate, ir.TTimestamp, ir.TBytes, ir.TJSON)
	case *ast.IntLit:
		return resolveDefaultLiteral(v.Pos, v.Value, field, ir.TInt32, ir.TInt64, ir.TFloat32, ir.TFloat64)
	case *ast.FloatLit:
		return resolveDefaultLiteral(v.Pos, v.Value, field, ir.TFloat32, ir.TFloat64)
	default:
		return diag.List{diag.Wrap("resolve", attr.Args[0].Pos, ErrInvalidAttribute, "unsupported default value")}
	}
}

func resolveDefaultCall(v *ast.CallValue, field *ir.Field) diag.List {
	if v.Name != "now" {
		return diag.List{diag.Wrap("resolve", v.Pos, ErrInvalidAttribute, "unknown default function %q", v.Name)}
	}

	if field.Type.Scalar != ir.TTimestamp {
		return diag.List{diag.Wrap("resolve", v.Pos, ErrInvalidAttribute, "now() default only valid on timestamp fields")}
	}

	field.Default = &ir.DefaultValue{Kind: "now", Pos: v.Pos}

	return nil
}

func resolveDefaultIdent(v *ast.IdentValue, field *ir.Field) diag.List {
	// The grammar has no boolean literal (Value = string | Number |
	// Duration | SetLit | CallOrIdent), so "@default(true)"/"@default(false)"
	// lex as a bare identifier rather than a distinct token kind. TBool
	// fields are special-cased here to still accept them.
	if field.Type.Scalar == ir.TBool {
		switch v.Name {
		case "true":
			field.Default = &ir.DefaultValue{Kind: "literal", Lit: true, Pos: v.Pos}
			return nil
		case "false":
			field.Default = &ir.DefaultValue{Kind: "literal", Lit: false, Pos: v.Pos}
			return nil
		default:
			return diag.List{diag.Wrap("resolve", v.Pos, ErrInvalidAttribute,
				"default value kind mismatches field type")}
		}
	}

	if field.Type.Scalar != ir.TEnum {
		return diag.List{diag.Wrap("resolve", v.Pos, ErrInvalidAttribute, "default value kind mismatches field type")}
	}

	for _, ev := range field.Type.EnumValues {
		if ev == v.Name {
			field.Default = &ir.DefaultValue{Kind: "literal", Lit: v.Name, Pos: v.Pos}
			return nil
		}
	}

	return diag.List{diag.Wrap("resolve", v.Pos, ErrUnresolvedReference, "unknown enum value %q for default", v.Name)}
}

// resolveDefaultLiteral sets field.Default when field's scalar is one of
// allowed, else reports a scalar-family mismatch.
func resolveDefaultLiteral(pos diag.Position, lit any, field *ir.Field, allowed ...ir.ScalarType) diag.List {
	if !scalarInSet(field.Type.Scalar, allowed...) {
		return diag.List{diag.Wrap("resolve", pos, ErrInvalidAttribute, "default value kind mismatches field type")}
	}

	field.Default = &ir.DefaultValue{Kind: "literal", Lit: lit, Pos: pos}

	return nil
}

// resolveEntitySchema resolves an entity's Schema string: explicit
// @schema(name) wins; else Module.Name if non-empty; else "public". It also
// reports any entity-level attribute other than "schema" as
// ErrInvalidAttribute.
func resolveEntitySchema(decl *ast.EntityDecl, module *ir.Module) (string, diag.List) {
	var diags diag.List

	schemaName := ""

	for _, attr := range decl.Attributes {
		if attr.Name != "schema" {
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "unknown entity attribute @%s", attr.Name))
			continue
		}

		name, d := resolveSchemaAttribute(attr)
		if d != nil {
			diags = append(diags, d)
			continue
		}

		schemaName = name
	}

	if schemaName != "" {
		return schemaName, diags
	}

	if module.Name != "" {
		return module.Name, diags
	}

	return "public", diags
}

// resolveSchemaAttribute decodes @schema(name)'s single argument, accepted
// as either a bare identifier or a string literal.
func resolveSchemaAttribute(attr *ast.Attribute) (string, *diag.Diagnostic) {
	if len(attr.Args) != 1 {
		return "", diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "@schema requires exactly one argument")
	}

	switch v := attr.Args[0].Value.(type) {
	case *ast.IdentValue:
		return v.Name, nil
	case *ast.StringLit:
		return v.Value, nil
	default:
		return "", diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "@schema argument must be an identifier or string")
	}
}

// isNumericScalar reports whether s is one of the numeric scalar types.
func isNumericScalar(s ir.ScalarType) bool {
	return scalarInSet(s, ir.TInt32, ir.TInt64, ir.TFloat32, ir.TFloat64)
}

// scalarInSet reports whether s is a member of set.
func scalarInSet(s ir.ScalarType, set ...ir.ScalarType) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}

	return false
}
