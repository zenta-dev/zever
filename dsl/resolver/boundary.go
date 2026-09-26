package resolver

import (
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// CheckCrossModule scans a resolved schema for cross-module references that
// are not allowed outside service calls. It checks relation targets and
// operation return types. Schedule dispatch cross-module is allowed by design
// (see resolver_schedule.go:51) and is intentionally not checked.
func CheckCrossModule(schema *ir.Schema) diag.List {
	var diags diag.List

	if schema == nil {
		return diags
	}

	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			for _, r := range e.Relations {
				if r.Target == nil || r.Target.Module == nil {
					continue
				}

				if r.Target.Module != e.Module {
					diags = append(diags, diag.Wrap("resolve", r.Pos, ErrCrossModule,
						"relation %s.%s -> %s crosses module boundary (%s -> %s); cross-module access must go through a service RPC, not a direct relation",
						e.Name, r.FieldName, r.Target.Name, moduleName(e.Module), moduleName(r.Target.Module)))
				}
			}
		}

		for _, svc := range m.Services {
			for _, op := range svc.Operations {
				for _, p := range op.Params {
					if p.Ref == nil {
						continue
					}

					paramModule := p.Ref.Module()
					if paramModule != nil && paramModule != svc.Module {
						diags = append(diags, diag.Wrap("resolve", p.Pos, ErrCrossModule,
							"rpc %s.%s param %s references %s from a different module (%s -> %s); expose it via that module's own service instead",
							svc.Name, op.Name, p.Name, p.Ref.Name(), moduleName(svc.Module), moduleName(paramModule)))
					}
				}

				if op.Returns == nil {
					continue
				}
				// Check return type's module vs service's module.
				retModule := op.Returns.Module()
				if retModule != nil && retModule != svc.Module {
					retName := op.Returns.Name()
					diags = append(diags, diag.Wrap("resolve", op.Pos, ErrCrossModule,
						"rpc %s.%s returns %s from a different module (%s -> %s); expose it via that module's own service instead",
						svc.Name, op.Name, retName, moduleName(svc.Module), moduleName(retModule)))
				}
				// Also check permission resource if any (cross-module resource).
				if op.Permission != nil && op.Permission.Resource != nil && op.Permission.Resource.Module != svc.Module {
					diags = append(diags, diag.Wrap("resolve", op.Permission.Pos, ErrCrossModule,
						"permission resource %s is from a different module (%s -> %s)",
						op.Permission.Resource.Name, moduleName(svc.Module), moduleName(op.Permission.Resource.Module)))
				}
			}
		}
	}

	return diags
}

func moduleName(m *ir.Module) string {
	if m == nil || m.Name == "" {
		return "public"
	}

	return m.Name
}
