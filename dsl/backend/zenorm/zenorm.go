package zenorm

import (
	"github.com/zenta-dev/zever/dsl/ir"
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
// schema.Modules entry holding at least one entity. Only entity fields and
// the named enums those fields reference are read -- see model.go's
// newEntityModel; messages and services are ignored.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte, len(schema.Modules))

	enums := indexEnums(schema)

	for _, m := range schema.Modules {
		if len(m.Entities) == 0 {
			continue
		}

		path, content, err := renderModuleFile(m, enums)
		if err != nil {
			return nil, err
		}

		out[path] = content
	}

	return out, nil
}

// indexEnums flattens every module's named enums into a single name ->
// *ir.Enum map. Named enums are global (any module may reference an enum
// declared in any other module), so every module file resolves referenced
// enum values through this shared index instead of only its own module.
func indexEnums(schema *ir.Schema) map[string]*ir.Enum {
	idx := make(map[string]*ir.Enum)

	if schema == nil {
		return idx
	}

	for _, m := range schema.Modules {
		for _, e := range m.Enums {
			idx[e.Name] = e
		}
	}

	return idx
}
