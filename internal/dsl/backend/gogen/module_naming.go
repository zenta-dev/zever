package gogen

import (
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// moduleNaming returns the generated Go package name and output-relative
// directory for m: a named module "billing" maps to package "billing" at
// directory "billing" (so "billing/types.go", "billing/service.go", ...).
//
// The implicit unnamed module maps to package "app" at directory "app",
// exactly matching zenorm's moduleNaming ("app" is a legal, non-reserved
// Go identifier and matches this repo's own examples/todo layout convention
// for "the application's default namespace").
func moduleNaming(m *ir.Module) (pkg, dir string) {
	name := "app"
	if m != nil && m.Name != "" {
		name = naming.SnakeCase(m.Name)
	}

	return name, name
}

// ModuleNaming is the exported form of moduleNaming, for callers outside
// this package (a schema-aware `generate server` command) that need to
// name the same generated package/directory a Generate call for m would
// produce, without duplicating the naming rule.
func ModuleNaming(m *ir.Module) (pkg, dir string) {
	return moduleNaming(m)
}

// ModuleNeedsAuthz reports whether m has at least one operation declaring
// auth: or permission:, exactly the same rule renderRegister (register.go)
// used to decide whether RegisterModule takes auth.Auth/permission.Checker
// params. A schema-aware `generate server` command calls this to know
// which shape of RegisterModule call to emit, without duplicating the
// per-operation rule.
func ModuleNeedsAuthz(m *ir.Module) bool {
	for _, svc := range m.Services {
		for _, op := range svc.Operations {
			if op.Auth != nil || op.Permission != nil {
				return true
			}
		}
	}

	return false
}
