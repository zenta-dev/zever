package zenorm

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/naming"
)

// moduleNaming returns the generated Go package name and output-relative
// file path for m: a named module "billing" maps to package "billing" at
// "orm/gen/billing/billing.go". The implicit unnamed module maps to
// package "app" at "orm/gen/app/app.go".
//
// name is validated as a path-safe single segment before use: naming.SnakeCase
// only lower-cases and inserts underscores, it never strips "/" or "..", so a
// module name carrying either (e.g. from a schema directory structure with an
// unusual "..".zen path) must be rejected here rather than trusted to produce
// a safe "orm/gen/<name>/<name>.go" path -- callers join this path onto a
// real output directory with no confinement check of their own.
func moduleNaming(m *ir.Module) (pkg, path string, err error) {
	name := "app"
	if m != nil && m.Name != "" {
		name = naming.SnakeCase(m.Name)
	}

	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", "", fmt.Errorf("zenorm: module name %q is not path-safe", name)
	}

	return name, fmt.Sprintf("orm/gen/%s/%s.go", name, name), nil
}
