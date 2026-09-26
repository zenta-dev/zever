package resolver

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// resolveValidateRules resolves every argument of one @validate(...)
// attribute into []ir.Validation, checking applicability against the given
// scalar type: format/min_len/max_len apply to TString only, gt/gte/lt/lte
// apply to numeric scalars only. format is further restricted to the fixed
// v1 set {email, url, uuid} — free-form validate is deferred.
//
// This is the single shared implementation behind @validate on both entity
// fields (resolver_entity.go's resolveFieldAttribute) and RPC params
// (resolver_service.go's resolveParams) — one applicability/decoding
// implementation, two call sites, so the two attachment points can never
// diverge in behavior.
func resolveValidateRules(attr *ast.Attribute, scalar ir.ScalarType) ([]ir.Validation, diag.List) {
	var (
		diags diag.List
		vals  []ir.Validation
	)

	for _, arg := range attr.Args {
		kind := arg.Name

		switch kind {
		case "format", "min_len", "max_len":
			if scalar != ir.TString {
				diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute,
					"validate kind %q only applies to string fields", kind))

				continue
			}
		case "gt", "gte", "lt", "lte":
			if !isNumericScalar(scalar) {
				diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute,
					"validate kind %q only applies to numeric fields", kind))

				continue
			}
		default:
			diags = append(diags, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute, "unknown validate kind %q", kind))
			continue
		}

		value, d := decodeValidateValue(kind, arg)
		if d != nil {
			diags = append(diags, d)
			continue
		}

		vals = append(vals, ir.Validation{
			Kind: kind,
			Args: map[string]any{"value": value},
			Pos:  arg.Pos,
		})
	}

	return vals, diags
}

// hasApplicableValidateKind reports whether scalar has at least one
// @validate kind that could ever apply to it -- mirrors, rather than
// duplicates, resolveValidateRules' own applicability switch immediately
// above: TString (format/min_len/max_len) and every numeric scalar
// (gt/gte/lt/lte) do, every other scalar (uuid, bool, timestamp, date,
// bytes, json, enum) does not. validateParamFields (resolver_opinions.go)
// uses this to exempt such a field from the "every request-position field
// must declare @validate" opinion: a field with no applicable kind is
// already fully type-safe as-is, and requiring @validate on it would be
// unsatisfiable no matter what the schema author writes.
func hasApplicableValidateKind(scalar ir.ScalarType) bool {
	return scalar == ir.TString || isNumericScalar(scalar)
}

// decodeValidateValue decodes one @validate argument's value, applying the
// fixed v1 format set for "format" and requiring a numeric literal for
// every other (already applicability-checked) kind.
func decodeValidateValue(kind string, arg *ast.Arg) (any, *diag.Diagnostic) {
	if kind == "format" {
		s, ok := arg.Value.(*ast.StringLit)
		if !ok {
			return nil, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute, "format value must be a string")
		}

		switch s.Value {
		case "email", "url", "uuid":
			return s.Value, nil
		default:
			return nil, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute,
				"unknown format %q, expected one of email, url, uuid", s.Value)
		}
	}

	switch v := arg.Value.(type) {
	case *ast.IntLit:
		return v.Value, nil
	case *ast.FloatLit:
		return v.Value, nil
	default:
		return nil, diag.Wrap("resolve", arg.Pos, ErrInvalidAttribute, "validate kind %q requires a numeric value", kind)
	}
}
