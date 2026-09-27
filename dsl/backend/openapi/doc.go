package openapi

import (
	"fmt"

	"github.com/zenta-dev/zever/dsl/ir"
)

// openAPIDoc is the root OpenAPI 3.0.3 document object, restricted to the
// fields this backend actually populates.
type openAPIDoc struct {
	OpenAPI    string              `json:"openapi"`
	Info       info                `json:"info"`
	Paths      map[string]pathItem `json:"paths"`
	Components components          `json:"components"`
}

type info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type components struct {
	Schemas         map[string]*schemaObject   `json:"schemas"`
	SecuritySchemes map[string]*securityScheme `json:"securitySchemes"`
}

type securityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
}

// pathItem maps an HTTP method (lowercase: get/post/put/patch/delete) to
// its operation.
type pathItem map[string]*operation

// qualifyFunc names a component schema for an entity/synthesized-request
// base name, scoped to the module that owns it. Per-module documents use
// the identity function (bare names, no collision risk within one
// module's own entities); the merged document always prefixes with the
// owning module's label, eliminating cross-module name collisions
// outright rather than detecting them.
type qualifyFunc func(name string, m *ir.Module) string

func bareQualify(name string, _ *ir.Module) string {
	return name
}

func mergedQualify(name string, m *ir.Module) string {
	return moduleLabel(m) + "_" + name
}

// docBuilder accumulates one OpenAPI document (paths + component schemas)
// while walking one or more modules. The same builder type serves both the
// per-module documents (one builder per module, bareQualify) and the
// merged document (one builder shared across every module, mergedQualify).
type docBuilder struct {
	doc     *openAPIDoc
	qualify qualifyFunc

	// pathOwner records, per "METHOD path" key, the "module.service.rpc"
	// identifier that first claimed it — so a second RPC declaring the
	// identical method+path is reported as a real conflict instead of
	// silently overwriting the first operation in doc.Paths.
	pathOwner map[string]string

	// schemaAdded guards against re-rendering the same component schema
	// name twice (e.g. an entity referenced as both a module entity and an
	// RPC's Returns value).
	schemaAdded map[string]bool

	// requestSchemaOwner records, per synthesized request-schema name, the
	// "<Service>.<RPCName>" that first claimed it -- see
	// addRequestSchema's doc comment for why this needs a real collision
	// check rather than schemaAdded's idempotent-skip.
	requestSchemaOwner map[string]string

	// enums is a name -> *ir.Enum lookup spanning every module of the
	// schema (named enums are global, so a field in any module may
	// reference an enum declared in any other), used by addEnumSchema to
	// find the enum's owning Module for qualify and its Values to render.
	enums map[string]*ir.Enum
}

func newDocBuilder(title string, qualify qualifyFunc, enums map[string]*ir.Enum) *docBuilder {
	return &docBuilder{
		doc: &openAPIDoc{
			OpenAPI: "3.0.3",
			Info:    info{Title: title, Version: "0.0.0"},
			Paths:   map[string]pathItem{},
			Components: components{
				Schemas: map[string]*schemaObject{},
				SecuritySchemes: map[string]*securityScheme{
					"bearerAuth": {Type: "http", Scheme: "bearer"},
				},
			},
		},
		qualify:            qualify,
		pathOwner:          map[string]string{},
		schemaAdded:        map[string]bool{},
		requestSchemaOwner: map[string]string{},
		enums:              enums,
	}
}

// addEntitySchema registers e's OpenAPI schema under its qualified
// component name (idempotent) and returns that name.
func (d *docBuilder) addEntitySchema(e *ir.Entity) string {
	name := d.qualify(e.Name, e.Module)

	if !d.schemaAdded[name] {
		d.doc.Components.Schemas[name] = d.renderEntitySchema(e)
		d.schemaAdded[name] = true
	}

	return name
}

// addEnumSchema registers a named enum's OpenAPI schema ({type: string,
// enum: [...]}) under its qualified component name (idempotent) and returns
// that name, so every field referencing the same named enum shares one
// component instead of each inlining its own copy. name is looked up in
// d.enums (built once from the whole schema, since named enums are global);
// an unknown name is unreachable given the resolver's invariants and
// returns "" defensively.
func (d *docBuilder) addEnumSchema(name string) string {
	enum, ok := d.enums[name]
	if !ok {
		return ""
	}

	qualified := d.qualify(enum.Name, enum.Module)

	if !d.schemaAdded[qualified] {
		d.doc.Components.Schemas[qualified] = &schemaObject{Type: "string", Enum: enum.Values}
		d.schemaAdded[qualified] = true
	}

	return qualified
}

// fieldSchema renders one field/param's ir.FieldType as an OpenAPI schema:
// a named enum (ft.EnumName != "") becomes a $ref to its shared component
// schema (registered via addEnumSchema); everything else -- including an
// anonymous inline enum(...), which has no shared component to reference --
// uses the plain renderFieldSchema mapping.
func (d *docBuilder) fieldSchema(ft ir.FieldType, validate []ir.Validation) *schemaObject {
	if ft.Scalar == ir.TEnum && ft.EnumName != "" {
		name := d.addEnumSchema(ft.EnumName)
		return &schemaObject{Ref: "#/components/schemas/" + name}
	}

	return renderFieldSchema(ft, validate)
}

// addMessageSchema registers m's OpenAPI schema under its qualified
// component name (idempotent) and returns that name. Messages never emit
// tables but do emit schemas.
//
// schemaAdded is set before rendering m's own schema (not after, unlike the
// idiom would suggest): a Ref-typed field renders via renderMessageSchema ->
// addTypeRefSchema -> addMessageSchema, so two messages that reference each
// other (directly, or through a longer cycle) would recurse into this
// function forever if the placeholder weren't already in place by the time
// the cycle closes. The $ref this and every other caller actually needs is
// just the qualified name, which is already known up front -- the schema
// object itself is filled in afterward, in place, once rendering returns.
func (d *docBuilder) addMessageSchema(m *ir.Message) string {
	name := d.qualify(m.Name, m.Module)

	if d.schemaAdded[name] {
		return name
	}

	d.schemaAdded[name] = true
	d.doc.Components.Schemas[name] = d.renderMessageSchema(m)

	return name
}

// addTypeRefSchema registers a TypeRef's underlying entity or message
// schema and returns its qualified name.
func (d *docBuilder) addTypeRefSchema(ref *ir.TypeRef) string {
	if ref == nil {
		return ""
	}

	if ref.Entity != nil {
		return d.addEntitySchema(ref.Entity)
	}

	if ref.Message != nil {
		return d.addMessageSchema(ref.Message)
	}

	return ""
}

// addRequestSchema registers a synthesized "<Service><RPCName>Request"
// schema (baseName already includes the service name and "Request" suffix,
// matching OperationID's own "<Service>_<RPCName>" namespacing) scoped to
// module m, and returns its qualified component name. owner identifies the
// RPC registering it ("<Service>.<RPCName>"), for the collision error
// below. Any param referencing an entity/message (p.Ref set) registers that
// type's own component schema (idempotent, via addTypeRefSchema) and is
// rendered as a $ref property rather than an inline scalar schema.
//
// Unlike addEntitySchema/addMessageSchema (where re-adding the same name is
// always safe -- it's always the same source entity/message re-rendering to
// identical content), a request-schema name collision from two DIFFERENT
// owners means two distinct RPCs would silently share one (wrong-for-one-
// of-them) component schema, so this errors instead -- the same "name both
// conflicting owners" contract addOperation already provides for
// path+method collisions.
func (d *docBuilder) addRequestSchema(baseName, owner string, m *ir.Module, params []*ir.Param) (string, error) {
	name := d.qualify(baseName, m)

	if prior, ok := d.requestSchemaOwner[name]; ok && prior != owner {
		return "", fmt.Errorf("openapi: request schema %q is declared by both %s and %s", name, prior, owner)
	}

	d.requestSchemaOwner[name] = owner
	d.doc.Components.Schemas[name] = d.renderRequestSchema(params)
	d.schemaAdded[name] = true

	return name, nil
}

// paramSchema renders one param's schema: a $ref to its referenced type's
// component schema (registering it first) when p.Ref is set, otherwise the
// plain scalar schema built from p.Type/p.Validate.
func (d *docBuilder) paramSchema(p *ir.Param) *schemaObject {
	if p.Ref != nil {
		name := d.addTypeRefSchema(p.Ref)
		return &schemaObject{Ref: "#/components/schemas/" + name}
	}

	return d.fieldSchema(p.Type, p.Validate)
}

// addOperation registers op under path/method, returning an error naming
// both conflicting owners if that exact method+path was already claimed by
// a different RPC.
func (d *docBuilder) addOperation(method, path string, op *operation, owner string) error {
	key := method + " " + path

	if prior, ok := d.pathOwner[key]; ok {
		return fmt.Errorf("openapi: %s %s is declared by both %s and %s", method, path, prior, owner)
	}

	d.pathOwner[key] = owner

	pi, ok := d.doc.Paths[path]
	if !ok {
		pi = pathItem{}
		d.doc.Paths[path] = pi
	}

	pi[httpMethodKey(method)] = op

	return nil
}

func httpMethodKey(method string) string {
	switch method {
	case "GET":
		return "get"
	case "POST":
		return "post"
	case "PUT":
		return "put"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}
