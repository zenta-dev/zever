package main

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// scalarDescription returns a human-readable one-liner for a builtin scalar
// name. Unknown names are returned unchanged.
func scalarDescription(name string) string {
	for _, c := range scalarTypeCandidates {
		if c.label != name {
			continue
		}

		if name == "enum" {
			return "inline enumeration"
		}

		return c.detail
	}

	return name
}

// fieldTypeHint renders a field's type together with a compact summary of
// its attributes, using only the AST (no resolved schema).
func fieldTypeHint(f *ast.FieldDecl) string {
	if f == nil {
		return ""
	}

	typeName := "?"
	if f.Type != nil {
		typeName = renderTypeExpr(f.Type)
	}

	var tags []string
	var defaultHint string

	for _, attr := range f.Attributes {
		if attr == nil {
			continue
		}

		switch attr.Name {
		case "primary":
			tags = append(tags, "primary")
		case "unique":
			tags = append(tags, "unique")
		case "default":
			values := make([]string, 0, len(attr.Args))
			for _, arg := range attr.Args {
				values = append(values, renderValue(arg.Value))
			}

			if len(values) > 0 {
				defaultHint = " (default: " + strings.Join(values, ", ") + ")"
			} else {
				defaultHint = " (default)"
			}
		}
	}

	result := typeName
	if defaultHint != "" {
		result += defaultHint
	}

	if len(tags) > 0 {
		result += " (" + strings.Join(tags, ", ") + ")"
	}

	return result
}

// relationTargetHint describes the resolved target of a relation, or reports
// it as unresolved when the schema does not contain it.
func relationTargetHint(schema *ir.Schema, rel *ast.RelationDecl) string {
	if rel == nil {
		return ""
	}

	target := findEntity(schema, rel.Target)
	if target == nil {
		return "unresolved entity " + rel.Target
	}

	return fmt.Sprintf("entity %s: %d field(s), %d relation(s)", target.Name, len(target.Fields), len(target.Relations))
}

// entitySummary renders a compact summary of an entity declaration.
func entitySummary(e *ast.EntityDecl) string {
	if e == nil {
		return ""
	}

	return fmt.Sprintf("entity %s: %d field(s), %d relation(s)", e.Name, len(e.Fields), len(e.Relations))
}

// errorCaseHint wraps the resolver's ValidErrorCodes lookup, reporting the
// HTTP status and gRPC name for a known code, or a fallback for unknowns.
func errorCaseHint(name string) string {
	code, ok := resolver.ValidErrorCodes[name]
	if !ok {
		return "unknown error code"
	}

	return fmt.Sprintf("HTTP %d, gRPC %s", code.HTTPStatus(), code.GRPCName())
}
