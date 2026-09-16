package resolver

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// buildEnumIndex flattens every module's already-resolved enums into a
// single name -> *ir.Enum map. Named enums are global (any module may
// reference an enum declared in any other module), so this is the one
// lookup every field-type resolution site consults.
func buildEnumIndex(modules map[string]*ir.Module) map[string]*ir.Enum {
	idx := make(map[string]*ir.Enum)

	for _, m := range modules {
		for _, e := range m.Enums {
			idx[e.Name] = e
		}
	}

	return idx
}

// resolveEnums is the enum declaration pass: it resolves every enum decl (in
// Pass 0's source-declaration order), validates its value list, and appends
// each resolved *ir.Enum to its owning Module's Enums slice. Runs before
// resolveEntities/resolveMessageFields/resolveServices so buildEnumIndex has
// every named enum available when field types are resolved.
func resolveEnums(decls []*ast.EnumDecl, declModule map[ast.Decl]*ir.Module) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			// Unreachable given Pass -1/0's invariants; guarded defensively.
			continue
		}

		enum, d := resolveEnum(decl, module)
		diags = append(diags, d...)

		if enum == nil {
			continue
		}

		module.Enums = append(module.Enums, enum)
	}

	return diags
}

// resolveEnum validates one enum decl's value list (at least one value, no
// duplicates) and returns the resolved *ir.Enum. A decl that fails
// validation still yields an *ir.Enum with whatever values it had (so a
// later reference to it doesn't cascade into a second, misleading
// "unresolved reference" diagnostic), except when it has zero values, which
// is unrecoverable.
func resolveEnum(decl *ast.EnumDecl, module *ir.Module) (*ir.Enum, diag.List) {
	if len(decl.Values) == 0 {
		return nil, diag.List{diag.Wrap("resolve", decl.Pos, ErrInvalidAttribute,
			"enum %q must declare at least one value", decl.Name)}
	}

	var diags diag.List

	seen := make(map[string]bool, len(decl.Values))

	for i, v := range decl.Values {
		if seen[v] {
			pos := decl.Pos
			if i < len(decl.ValuePos) {
				pos = decl.ValuePos[i]
			}

			diags = append(diags, diag.Wrap("resolve", pos, ErrInvalidAttribute,
				"duplicate value %q in enum %q", v, decl.Name))

			continue
		}

		seen[v] = true
	}

	enum := &ir.Enum{
		Name:       decl.Name,
		Values:     decl.Values,
		Module:     module,
		Pos:        decl.Pos,
		DocComment: decl.DocComment,
	}

	return enum, diags
}
