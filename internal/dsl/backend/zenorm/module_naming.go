package zenorm

import (
	"fmt"

	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/naming"
)

// moduleNaming returns the generated Go package name and output-relative
// file path for m: a named module "billing" maps to package "billing" at
// "orm/gen/billing/billing.go". The implicit unnamed module maps to
// package "app" at "orm/gen/app/app.go".
func moduleNaming(m *ir.Module) (pkg, path string) {
	name := "app"
	if m != nil && m.Name != "" {
		name = naming.SnakeCase(m.Name)
	}

	return name, fmt.Sprintf("orm/gen/%s/%s.go", name, name)
}
