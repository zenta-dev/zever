package mcp

import (
	"fmt"

	"github.com/zenta-dev/zever/dsl/ir"
)

// toolManifest is one model-callable tool definition.
type toolManifest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Paginated    bool           `json:"paginated,omitempty"`
}

// serviceManifest groups one service's tools with the shared definitions
// their schemas reference, so each service manifest stands alone.
type serviceManifest struct {
	Service     string         `json:"service"`
	Module      string         `json:"module"`
	Tools       []toolManifest `json:"tools"`
	Definitions map[string]any `json:"definitions,omitempty"`
}

// moduleManifest lists one module's service manifests.
type moduleManifest struct {
	Module   string            `json:"module"`
	Services []serviceManifest `json:"services"`
}

// mergedManifest is the whole-schema tool catalog.
type mergedManifest struct {
	Name     string            `json:"name"`
	Services []serviceManifest `json:"services"`
}

// renderer accumulates shared definitions while rendering one service.
type renderer struct {
	enums      map[string][]string
	enumOwners map[string]string
	defs       map[string]map[string]any
	seen       map[string]bool
}

// renderService renders one service's operations as MCP tools. Definitions
// are qualified "<module>_<Name>" by the type's owning module so identical
// type names in different modules never collide.
func renderService(m *ir.Module, svc *ir.Service, enums map[string][]string, enumOwners map[string]string) (serviceManifest, error) {
	r := &renderer{
		enums:      enums,
		enumOwners: enumOwners,
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}

	sm := serviceManifest{Service: svc.Name, Module: moduleLabel(m), Tools: []toolManifest{}}

	for _, op := range svc.Operations {
		tool, err := r.renderOperation(m, svc, op)
		if err != nil {
			return serviceManifest{}, err
		}

		sm.Tools = append(sm.Tools, tool)
	}

	if len(r.defs) > 0 {
		sm.Definitions = make(map[string]any, len(r.defs))
		for name, def := range r.defs {
			sm.Definitions[name] = def
		}
	}

	return sm, nil
}

// renderOperation renders one RPC as an MCP tool. The tool name is namespaced
// "<module>_<Service>_<Operation>".
func (r *renderer) renderOperation(m *ir.Module, svc *ir.Service, op *ir.Operation) (toolManifest, error) {
	props := make(map[string]any, len(op.Params))

	for _, p := range op.Params {
		schema, err := r.paramSchema(m, p)
		if err != nil {
			return toolManifest{}, err
		}

		props[p.Name] = schema
	}

	if op.Paginated {
		props["cursor"] = map[string]any{"type": "string"}
		props["limit"] = map[string]any{"type": "integer"}
	}

	tool := toolManifest{
		Name:        fmt.Sprintf("%s_%s_%s", moduleLabel(m), svc.Name, op.Name),
		Description: op.DocComment,
		InputSchema: map[string]any{"type": "object", "properties": props},
		Paginated:   op.Paginated,
	}

	if tool.Description == "" {
		tool.Description = fmt.Sprintf("%s.%s RPC", svc.Name, op.Name)
	}

	if op.Returns != nil {
		out, err := r.typeRefSchema(m, op.Returns)
		if err != nil {
			return toolManifest{}, err
		}

		if op.Paginated {
			out = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items":       map[string]any{"type": "array", "items": out},
					"next_cursor": map[string]any{"type": "string"},
				},
			}
		}

		tool.OutputSchema = out
	}

	return tool, nil
}

// paramSchema renders one RPC param as a JSON Schema value.
func (r *renderer) paramSchema(m *ir.Module, p *ir.Param) (map[string]any, error) {
	if p.Ref != nil {
		return r.typeRefSchema(m, p.Ref)
	}

	return r.fieldSchema(m, p.Type)
}

// fieldSchema renders one scalar or enum field type as a JSON Schema value.
// Named enums render as $refs to a shared string-enum definition qualified
// by the enum's owning module; validation overlays are intentionally not
// mapped: the manifest describes shapes for tool selection, not request
// validation.
func (r *renderer) fieldSchema(m *ir.Module, ft ir.FieldType) (map[string]any, error) {
	if ft.Scalar == ir.TEnum {
		if ft.EnumName == "" {
			return map[string]any{"type": "string", "enum": ft.EnumValues}, nil
		}

		values, ok := r.enums[ft.EnumName]
		if !ok {
			return nil, fmt.Errorf("mcp: unknown enum %q: %w", ft.EnumName, ErrInternalInvariant)
		}

		owner, ok := r.enumOwners[ft.EnumName]
		if !ok {
			owner = moduleLabel(m)
		}

		name := owner + "_" + ft.EnumName
		if !r.seen[name] {
			r.seen[name] = true
			r.defs[name] = map[string]any{"type": "string", "enum": values}
		}

		return map[string]any{"$ref": "#/definitions/" + name}, nil
	}

	if ft.Scalar == ir.TJSON {
		return map[string]any{}, nil
	}

	return map[string]any{"type": scalarJSONType(ft.Scalar)}, nil
}

// typeRefSchema renders an entity or message reference as a $ref, registering
// the referenced type's definition first. Definitions are qualified by the
// referenced type's owning module. Cycles terminate via the seen set:
// a type under construction is already marked, so recursive references resolve
// to its (eventually complete) definition.
func (r *renderer) typeRefSchema(m *ir.Module, ref *ir.TypeRef) (map[string]any, error) {
	if ref == nil {
		return nil, fmt.Errorf("mcp: nil type reference: %w", ErrInternalInvariant)
	}

	if ref.Entity != nil {
		name := qualify(ownerModule(m, ref), ref.Entity.Name)

		if !r.seen[name] {
			r.seen[name] = true

			def, err := r.entitySchema(m, ref.Entity)
			if err != nil {
				return nil, err
			}

			r.defs[name] = def
		}

		return map[string]any{"$ref": "#/definitions/" + name}, nil
	}

	if ref.Message != nil {
		name := qualify(ownerModule(m, ref), ref.Message.Name)

		if !r.seen[name] {
			r.seen[name] = true

			def, err := r.messageSchema(m, ref.Message)
			if err != nil {
				return nil, err
			}

			r.defs[name] = def
		}

		return map[string]any{"$ref": "#/definitions/" + name}, nil
	}

	return nil, fmt.Errorf("mcp: empty type reference: %w", ErrInternalInvariant)
}

// ownerModule returns the referenced type's owning module, falling back to
// the caller's module when the reference carries none.
func ownerModule(m *ir.Module, ref *ir.TypeRef) *ir.Module {
	if ref.Entity != nil && ref.Entity.Module != nil {
		return ref.Entity.Module
	}

	if ref.Message != nil && ref.Message.Module != nil {
		return ref.Message.Module
	}

	return m
}

// entitySchema renders an entity as an object schema: one property per field,
// primary fields required. Relations are excluded.
func (r *renderer) entitySchema(m *ir.Module, e *ir.Entity) (map[string]any, error) {
	props := make(map[string]any, len(e.Fields))

	var required []string

	for _, f := range e.Fields {
		var schema map[string]any

		var err error
		if f.Ref != nil {
			schema, err = r.typeRefSchema(m, f.Ref)
		} else {
			schema, err = r.fieldSchema(m, f.Type)
		}

		if err != nil {
			return nil, err
		}

		props[f.Name] = schema

		if f.Primary && !f.Optional {
			required = append(required, f.Name)
		}
	}

	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}

	return out, nil
}

// messageSchema renders a message as an object schema.
func (r *renderer) messageSchema(m *ir.Module, msg *ir.Message) (map[string]any, error) {
	props := make(map[string]any, len(msg.Fields))

	for _, f := range msg.Fields {
		var schema map[string]any

		var err error
		if f.Ref != nil {
			schema, err = r.typeRefSchema(m, f.Ref)
		} else {
			schema, err = r.fieldSchema(m, f.Type)
		}

		if err != nil {
			return nil, err
		}

		props[f.Name] = schema
	}

	return map[string]any{"type": "object", "properties": props}, nil
}

// qualify namespaces a definition name by its owning module.
func qualify(m *ir.Module, name string) string {
	return moduleLabel(m) + "_" + name
}

// scalarJSONType maps a scalar to its JSON Schema type name. TJSON is listed
// for exhaustiveness but never reaches here: fieldSchema returns an
// unconstrained schema for TJSON before calling scalarJSONType.
func scalarJSONType(s ir.ScalarType) string {
	switch s {
	case ir.TUUID, ir.TString, ir.TTimestamp, ir.TDate, ir.TBytes:
		return "string"
	case ir.TInt32, ir.TInt64:
		return "integer"
	case ir.TFloat32, ir.TFloat64:
		return "number"
	case ir.TBool:
		return "boolean"
	case ir.TEnum:
		return "string"
	case ir.TJSON:
		return "string"
	default:
		return "string"
	}
}
