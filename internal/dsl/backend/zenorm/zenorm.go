package zenorm

import (
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Backend renders a resolved schema to per-module Go orm query-builder
// source files.
type Backend struct{}

// New returns a new zenorm Backend.
func New() *Backend {
	return &Backend{}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "zenorm"
}

// Generate renders one "orm/gen/<module>/<module>.go" file per
// schema.Modules entry holding at least one entity. Messages, services and
// enums are not read by this backend at all -- see model.go's
// newEntityModel, which only reads e.Fields.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte, len(schema.Modules))

	for _, m := range schema.Modules {
		if len(m.Entities) == 0 {
			continue
		}

		path, content, err := renderModuleFile(m)
		if err != nil {
			return nil, err
		}

		out[path] = content
	}

	return out, nil
}
