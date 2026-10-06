package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/zenta-dev/zever/dsl/ir"
)

// Backend renders a resolved schema to MCP tool manifests.
type Backend struct{}

// New returns a new mcp Backend.
func New() *Backend {
	return &Backend{}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "mcp"
}

// Generate renders one "<module>/mcp.json" file per schema.Modules entry,
// each listing that module's services as MCP tools, plus one merged "mcp.json"
// (root, no module prefix) covering every module. Tool names are namespaced
// "<module>_<Service>_<Operation>" so identical service names in different
// modules never collide.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	if schema == nil {
		return nil, fmt.Errorf("mcp: nil schema: %w", ErrInternalInvariant)
	}

	out := make(map[string][]byte, len(schema.Modules)+1)
	merged := mergedManifest{Name: "zever", Services: []serviceManifest{}}

	values, owners := collectEnums(schema)

	for _, m := range schema.Modules {
		mod := moduleManifest{Module: moduleLabel(m), Services: []serviceManifest{}}

		for _, svc := range m.Services {
			sm, err := renderService(m, svc, values, owners)
			if err != nil {
				return nil, err
			}

			mod.Services = append(mod.Services, sm)
			merged.Services = append(merged.Services, sm)
		}

		content, err := json.MarshalIndent(mod, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal %s manifest: %w", moduleLabel(m), err)
		}

		out[moduleLabel(m)+"/mcp.json"] = content
	}

	mergedContent, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal merged manifest: %w", err)
	}

	out["mcp.json"] = mergedContent

	return out, nil
}

// moduleLabel returns m's manifest label: "default" for the implicit unnamed
// module, else m.Name.
func moduleLabel(m *ir.Module) string {
	if m.Name == "" {
		return "default"
	}

	return m.Name
}

// collectEnums flattens every module's named enums into a values lookup and
// an owning-module lookup spanning the whole schema, since named enums
// resolve globally rather than per module.
func collectEnums(schema *ir.Schema) (values map[string][]string, owners map[string]string) {
	values = make(map[string][]string)
	owners = make(map[string]string)

	for _, m := range schema.Modules {
		for _, e := range m.Enums {
			values[e.Name] = e.Values
			owners[e.Name] = moduleLabel(m)
		}
	}

	return values, owners
}
