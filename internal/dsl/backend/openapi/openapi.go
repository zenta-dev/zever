// Package openapi implements the "openapi" backend.Backend: it renders a
// resolved *ir.Schema into OpenAPI 3.0.3 JSON specs, one per ir.Module plus
// one merged spec covering every module's services, from the schema's
// Service/RPC/HTTP declarations.
package openapi

import (
	"encoding/json"
	"fmt"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// Backend renders a resolved schema to OpenAPI 3.0.3 JSON documents.
type Backend struct{}

// New returns a new openapi Backend.
func New() *Backend {
	return &Backend{}
}

// Name returns the backend's identifier.
func (b *Backend) Name() string {
	return "openapi"
}

// Generate renders one "<module-or-\"default\">/openapi.json" file per
// schema.Modules entry, plus one merged "openapi.json" (root, no module
// prefix) covering every module's paths and (module-qualified) component
// schemas. Two different modules declaring the identical HTTP method+path
// is reported as an error naming both conflicting RPCs, rather than
// letting one silently overwrite the other in the merged spec.
func (b *Backend) Generate(schema *ir.Schema) (map[string][]byte, error) {
	out := make(map[string][]byte, len(schema.Modules)+1)

	enums := collectEnums(schema)

	merged := newDocBuilder("zen-go API", mergedQualify, enums)

	for _, m := range schema.Modules {
		moduleDoc := newDocBuilder(moduleLabel(m), bareQualify, enums)

		if err := populateDoc(moduleDoc, m); err != nil {
			return nil, err
		}

		content, err := json.MarshalIndent(moduleDoc.doc, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("openapi: marshal %s spec: %w", moduleLabel(m), err)
		}

		out[moduleLabel(m)+"/openapi.json"] = content

		if err := populateDoc(merged, m); err != nil {
			return nil, err
		}
	}

	mergedContent, err := json.MarshalIndent(merged.doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("openapi: marshal merged spec: %w", err)
	}

	out["openapi.json"] = mergedContent

	return out, nil
}

// collectEnums flattens every module's named enums into one name -> *ir.Enum
// lookup spanning the whole schema, since named enums are resolved globally
// (see resolver_enum.go) rather than scoped to the module that declared
// them.
func collectEnums(schema *ir.Schema) map[string]*ir.Enum {
	enums := make(map[string]*ir.Enum)

	for _, m := range schema.Modules {
		for _, e := range m.Enums {
			enums[e.Name] = e
		}
	}

	return enums
}

// moduleLabel returns m's file-path/title/qualifier label: "default" for
// the implicit unnamed module, else m.Name.
func moduleLabel(m *ir.Module) string {
	if m.Name == "" {
		return "default"
	}

	return m.Name
}

// populateDoc registers every entity and message of m as component schemas and every
// HTTP-bound RPC of m's services as a path+operation on d. Messages never emit tables but do emit schemas and return types.
func populateDoc(d *docBuilder, m *ir.Module) error {
	for _, e := range m.Entities {
		d.addEntitySchema(e)
	}

	for _, msg := range m.Messages {
		d.addMessageSchema(msg)
	}

	for _, svc := range m.Services {
		for _, rpc := range svc.Operations {
			http, ok := findHTTPTransport(rpc.Transports)
			if !ok {
				continue
			}

			op := buildOperation(svc, rpc, http, d)
			owner := fmt.Sprintf("%s.%s.%s", moduleLabel(m), svc.Name, rpc.Name)

			if err := d.addOperation(http.Method, http.Path, op, owner); err != nil {
				return err
			}
		}
	}

	return nil
}
