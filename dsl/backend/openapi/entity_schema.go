package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/dsl/ir"
)

// JSONSchemaForEntity renders one named entity as a self-contained JSON
// Schema object for the given module label (use "default" for the implicit
// unnamed module). The result references its own definitions section instead
// of OpenAPI component paths, so agents can feed it directly to structured
// output (for example agent.GenerateStructured).
func JSONSchemaForEntity(s *ir.Schema, module, name string) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("openapi: JSONSchemaForEntity: nil schema: %w", ErrInternalInvariant)
	}

	var (
		target *ir.Entity
		owner  *ir.Module
	)

	for _, m := range s.Modules {
		if moduleLabel(m) != module {
			continue
		}

		for _, e := range m.Entities {
			if e.Name == name {
				target = e
				owner = m
			}
		}
	}

	if target == nil {
		return nil, fmt.Errorf("openapi: entity %q not found in module %q", name, module)
	}

	d := newDocBuilder(moduleLabel(owner), bareQualify, collectEnums(s))

	schemaName, err := d.addEntitySchema(target)
	if err != nil {
		return nil, err
	}

	defs := make(map[string]any, len(d.doc.Components.Schemas))

	for cname, cs := range d.doc.Components.Schemas {
		raw, err := json.Marshal(cs)
		if err != nil {
			return nil, err
		}

		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}

		rewriteComponentRefs(m)
		defs[cname] = m
	}

	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$ref":        "#/definitions/" + schemaName,
		"definitions": defs,
	}, nil
}

// rewriteComponentRefs rewrites "#/components/schemas/<Name>" references to
// "#/definitions/<Name>" in place so the schema stands alone.
func rewriteComponentRefs(m map[string]any) {
	for k, v := range m {
		switch t := v.(type) {
		case string:
			if k == "$ref" {
				if name, ok := strings.CutPrefix(t, "#/components/schemas/"); ok {
					m[k] = "#/definitions/" + name
				}
			}
		case map[string]any:
			rewriteComponentRefs(t)
		case []any:
			for _, e := range t {
				if em, ok := e.(map[string]any); ok {
					rewriteComponentRefs(em)
				}
			}
		}
	}
}
