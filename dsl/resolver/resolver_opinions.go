package resolver

import (
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// validateEverythingIn enforces the v-next "every request MUST be
// validated" opinion: for every service operation, for every param that
// resolves to an entity/message TypeRef (request position, i.e. an
// ir.Param with Ref set), every field of that referenced type must carry
// at least one @validate rule. A plain scalar param carries no such
// requirement itself (there is no field to attach @validate to besides the
// param, and @validate directly on a ref param is rejected earlier, in
// resolveRefParam).
//
// A message/entity used only as a Returns type (never as a param) is
// exempt: this pass only ever walks Operation.Params, never Operation.Returns.
// If the same type is used as both a param (somewhere) and a Returns type
// (elsewhere), the request-position rule still applies wherever it is used
// as a param -- this pass reports a missing-@validate diagnostic once per
// (operation, param, field) it finds, not deduplicated across operations,
// mirroring how CheckCrossModule reports once per offending reference
// rather than once per offending type.
func validateEverythingIn(schema *ir.Schema) diag.List {
	var diags diag.List

	if schema == nil {
		return diags
	}

	for _, m := range schema.Modules {
		for _, svc := range m.Services {
			for _, op := range svc.Operations {
				for _, p := range op.Params {
					diags = append(diags, validateParamFields(svc, op, p)...)
				}
			}
		}
	}

	return diags
}

// validateParamFields checks one request-position param's referenced
// type's fields (if p.Ref is set; a no-op for scalar params).
func validateParamFields(svc *ir.Service, op *ir.Operation, p *ir.Param) diag.List {
	if p.Ref == nil {
		return nil
	}

	var fields []*ir.Field

	switch {
	case p.Ref.Entity != nil:
		fields = p.Ref.Entity.Fields
	case p.Ref.Message != nil:
		fields = p.Ref.Message.Fields
	default:
		return nil
	}

	visited := map[*ir.Message]bool{}
	if p.Ref.Message != nil {
		visited[p.Ref.Message] = true
	}

	return validateFieldsIn(svc, op, p.Name, p.Ref.Name(), fields, visited, 0)
}

// maxValidateFieldsDepth bounds validateFieldsIn's recursion depth,
// mirroring parser_attribute.go's maxValueDepth: the visited map only
// catches a cycle (a message/entity chain that loops back on itself), not a
// long cycle-free chain of thousands of distinct declared types. A large
// generated or adversarial schema with a deep Ref chain must fail with a
// diagnostic here, never a stack-overflow panic.
const maxValidateFieldsDepth = 200

// validateFieldsIn walks one request-position type's fields (typeName names
// that type, for diagnostic text): a field with an applicable @validate kind
// (per hasApplicableValidateKind) must carry at least one @validate rule; a
// field whose scalar type has no applicable kind at all (uuid, bool,
// timestamp, date, bytes, json, enum -- see hasApplicableValidateKind) is
// exempt, since it is already fully type-safe and no @validate declaration
// could ever satisfy the rule. A field that is itself a Ref (a message
// embedding another entity/message) recurses into that referenced type's
// own fields instead of requiring/checking @validate on the field itself
// (mirroring resolveRefFieldForMessage's rejection of @validate directly on
// a ref-typed field) -- visited guards against a message that transitively
// (directly or through a cycle of messages) references itself, so a
// self-referential or mutually-referential message chain terminates rather
// than recursing forever.
func validateFieldsIn(
	svc *ir.Service, op *ir.Operation, paramName, typeName string, fields []*ir.Field, visited map[*ir.Message]bool, depth int,
) diag.List {
	var diags diag.List

	if depth > maxValidateFieldsDepth {
		diags = append(diags, diag.Wrap("resolve", diag.Position{}, ErrMissingValidation,
			"rpc %s.%s param %s: field reference chain exceeds %d levels (possible runaway Ref chain)",
			svc.Name, op.Name, paramName, maxValidateFieldsDepth))

		return diags
	}

	for _, f := range fields {
		if f.Ref != nil {
			switch {
			case f.Ref.Message != nil:
				if visited[f.Ref.Message] {
					continue
				}

				visited[f.Ref.Message] = true

				diags = append(diags, validateFieldsIn(svc, op, paramName, f.Ref.Name(), f.Ref.Message.Fields, visited, depth+1)...)
			case f.Ref.Entity != nil:
				diags = append(diags, validateFieldsIn(svc, op, paramName, f.Ref.Name(), f.Ref.Entity.Fields, visited, depth+1)...)
			}

			continue
		}

		if len(f.Validate) > 0 {
			continue
		}

		if !hasApplicableValidateKind(f.Type.Scalar) {
			continue
		}

		diags = append(diags, diag.Wrap("resolve", f.Pos, ErrMissingValidation,
			"rpc %s.%s param %s: field %s.%s must declare @validate (every request-position field must be validated)",
			svc.Name, op.Name, paramName, typeName, f.Name))
	}

	return diags
}

// secureEverything ensures every RPC operation declares auth or permission
// -- a hard compile-time opinion, per the v-next "every request must be
// secure" decision (secure-by-default: an operation with no Auth/Permission
// policy fails to compile). The diagnostic wraps ErrMissingAuth at the
// default SeverityError, same as every other resolver diagnostic, so
// diag.List.HasErrors() reports true and zever compile fails.
func secureEverything(schema *ir.Schema) diag.List {
	var diags diag.List

	if schema == nil {
		return diags
	}

	for _, m := range schema.Modules {
		for _, svc := range m.Services {
			for _, op := range svc.Operations {
				if op.Auth == nil && op.Permission == nil && !op.AuthDeclared {
					diags = append(diags, diag.Wrap("resolve", op.Pos, ErrMissingAuth,
						"rpc %s.%s requires auth or permission (secure-by-default)", svc.Name, op.Name))
				}
			}
		}
	}

	return diags
}
