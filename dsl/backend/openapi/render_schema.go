package openapi

import (
	"fmt"

	"github.com/zenta-dev/zever/dsl/ir"
)

// schemaObject is a minimal hand-rolled OpenAPI 3.0.3 Schema Object,
// covering exactly the shapes this backend emits: plain scalars
// ({type, format}), string enums, and object schemas with named
// properties (entities and synthesized request messages).
type schemaObject struct {
	Ref        string                   `json:"$ref,omitempty"`
	Type       string                   `json:"type,omitempty"`
	Format     string                   `json:"format,omitempty"`
	Enum       []string                 `json:"enum,omitempty"`
	Properties map[string]*schemaObject `json:"properties,omitempty"`
	Required   []string                 `json:"required,omitempty"`
	Nullable   bool                     `json:"nullable,omitempty"`
	// Items is the array-item schema, set only when Type == "array" (used to
	// render a paginated operation's "items" property as an array of the
	// page's entity schema).
	Items *schemaObject `json:"items,omitempty"`
	// MinLength/MaxLength mirror a @validate(min_len:/max_len:) rule.
	MinLength *int `json:"minLength,omitempty"`
	MaxLength *int `json:"maxLength,omitempty"`
	// Minimum/Maximum mirror a @validate(gt:/gte:/lt:/lte:) rule; Exclusive*
	// distinguishes gt/lt (exclusive) from gte/lte (inclusive), matching
	// OpenAPI 3.0's boolean-modifier form (not the draft-2020-12 numeric
	// exclusiveMinimum/Maximum form, since this backend targets OpenAPI
	// 3.0.3 -- see the package doc).
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum bool     `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum bool     `json:"exclusiveMaximum,omitempty"`
}

// applyValidation overlays field's @validate rules onto schema as the
// matching JSON Schema constraint keyword(s): format (overriding the plain
// scalar format for the fixed v1 {email, url, uuid} set, mapped to their
// JSON-Schema/OpenAPI-standard format names), min_len/max_len ->
// minLength/maxLength, gt/gte/lt/lte -> minimum/maximum (+ the matching
// exclusive flag). A field/param with no @validate rules is left untouched.
// An unexpected validation kind or non-numeric value is a resolver-invariant
// violation and returns ErrInternalInvariant rather than silently emitting a
// schema with no constraint.
func applyValidation(schema *schemaObject, rules []ir.Validation) error {
	for _, v := range rules {
		value := v.Args["value"]

		switch v.Kind {
		case ir.ValidationFormat:
			if s, ok := value.(string); ok {
				schema.Format = openAPIFormatName(s)
			}
		case ir.ValidationMinLen:
			n, err := toInt64(value)
			if err != nil {
				return err
			}
			nInt := int(n)
			schema.MinLength = &nInt
		case ir.ValidationMaxLen:
			n, err := toInt64(value)
			if err != nil {
				return err
			}
			nInt := int(n)
			schema.MaxLength = &nInt
		case ir.ValidationGT:
			f, err := toFloat64(value)
			if err != nil {
				return err
			}
			schema.Minimum = &f
			schema.ExclusiveMinimum = true
		case ir.ValidationGTE:
			f, err := toFloat64(value)
			if err != nil {
				return err
			}
			schema.Minimum = &f
		case ir.ValidationLT:
			f, err := toFloat64(value)
			if err != nil {
				return err
			}
			schema.Maximum = &f
			schema.ExclusiveMaximum = true
		case ir.ValidationLTE:
			f, err := toFloat64(value)
			if err != nil {
				return err
			}
			schema.Maximum = &f
		default:
			return fmt.Errorf("openapi: applyValidation: unexpected validation kind %q: %w", v.Kind, ErrInternalInvariant)
		}
	}

	return nil
}

// openAPIFormatName maps a @validate(format: "...") value to its
// JSON-Schema/OpenAPI-standard format name -- "url" is spelled "uri" in the
// standard vocabulary, email/uuid already match.
func openAPIFormatName(format string) string {
	if format == "url" {
		return "uri"
	}

	return format
}

// toInt64/toFloat64 convert a decoded @validate numeric argument (always
// int64 or float64, per resolver.decodeValidateValue) to the requested
// numeric type. A value of any other dynamic type means that invariant
// broke upstream: returning ErrInternalInvariant here (matching gogen's
// numericLiteral) fails loudly instead of silently emitting a bogus
// minLength:0/minimum:0 (etc.) constraint into generated OpenAPI with no
// diagnostic anywhere.
func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("openapi: toInt64: unexpected value type %T: %w", v, ErrInternalInvariant)
	}
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("openapi: toFloat64: unexpected value type %T: %w", v, ErrInternalInvariant)
	}
}

// scalarOpenAPI maps a DSL scalar type to its OpenAPI {type, format} pair.
// Switches over every ir.ScalarType case explicitly (mirrors the proto
// backend's protoScalar) so a new scalar type added later fails the
// exhaustive-mapping test loudly instead of silently mapping to "". TEnum
// is never passed here in practice (see renderFieldSchema), but is covered
// for exhaustiveness the same way protoScalar covers it.
func scalarOpenAPI(t ir.ScalarType) (typ, format string) {
	switch t {
	case ir.TUUID:
		return "string", "uuid"
	case ir.TString:
		return "string", ""
	case ir.TInt32:
		return "integer", "int32"
	case ir.TInt64:
		return "integer", "int64"
	case ir.TFloat32:
		return "number", "float"
	case ir.TFloat64:
		return "number", "double"
	case ir.TBool:
		return "boolean", ""
	case ir.TTimestamp:
		return "string", "date-time"
	case ir.TDate:
		return "string", "date"
	case ir.TBytes:
		return "string", "byte"
	case ir.TJSON:
		return "object", ""
	case ir.TEnum:
		return "string", ""
	}

	return "", ""
}

// renderFieldSchema renders one field/param's ir.FieldType as an OpenAPI
// schema object: an enum field becomes {type: string, enum: [...]} using
// its declared values, everything else uses the scalarOpenAPI mapping. Any
// @validate rules declared alongside the field/param are overlaid as the
// matching JSON Schema constraint keyword(s) via applyValidation, whose
// resolver-invariant violation is returned to the caller.
func renderFieldSchema(ft ir.FieldType, validate []ir.Validation) (*schemaObject, error) {
	var schema *schemaObject

	if ft.Scalar == ir.TEnum {
		schema = &schemaObject{Type: "string", Enum: ft.EnumValues}
	} else {
		typ, format := scalarOpenAPI(ft.Scalar)
		schema = &schemaObject{Type: typ, Format: format}
	}

	if err := applyValidation(schema, validate); err != nil {
		return nil, err
	}

	return schema, nil
}

// renderEntitySchema renders an entity as an OpenAPI object schema: one
// property per field (relations are excluded, matching the proto backend's
// message rendering). Primary fields are marked required. A field declared
// optional (`field: type?`) is excluded from required and marked
// `nullable: true`, folding "omittable in a request" and "NULL in the
// database" into the DSL's single Optional concept.
func (d *docBuilder) renderEntitySchema(e *ir.Entity) (*schemaObject, error) {
	obj := &schemaObject{
		Type:       "object",
		Properties: make(map[string]*schemaObject, len(e.Fields)),
	}

	for _, f := range e.Fields {
		fieldSchema, err := d.fieldSchema(f.Type, f.Validate)
		if err != nil {
			return nil, err
		}
		fieldSchema.Nullable = f.Optional
		obj.Properties[f.Name] = fieldSchema

		if f.Primary && !f.Optional {
			obj.Required = append(obj.Required, f.Name)
		}
	}

	return obj, nil
}

// renderMessageSchema renders a message as an OpenAPI object schema.
// Messages are otherwise scalar-only and never have primary/relation
// semantics, but a field may itself reference another entity/message
// (f.Ref set, mirroring how ir.Param.Ref renders via docBuilder.paramSchema)
// -- such a field renders as a $ref to that type's own registered component
// schema (registering it first, via addTypeRefSchema) instead of an inline
// scalar schema. Messages never emit tables but do emit schemas.
//
// A method on *docBuilder (not a free function) because rendering a Ref
// field needs the same schema registry/qualify machinery paramSchema uses.
func (d *docBuilder) renderMessageSchema(m *ir.Message) (*schemaObject, error) {
	obj := &schemaObject{
		Type:       "object",
		Properties: make(map[string]*schemaObject, len(m.Fields)),
	}

	for _, f := range m.Fields {
		var fieldSchema *schemaObject

		if f.Ref != nil {
			name, err := d.addTypeRefSchema(f.Ref)
			if err != nil {
				return nil, err
			}
			fieldSchema = &schemaObject{Ref: "#/components/schemas/" + name}
		} else {
			var err error
			fieldSchema, err = d.fieldSchema(f.Type, f.Validate)
			if err != nil {
				return nil, err
			}
			fieldSchema.Nullable = f.Optional
		}

		obj.Properties[f.Name] = fieldSchema
	}

	return obj, nil
}

// renderRequestSchema renders an RPC's non-path params as a synthesized
// "<RPCName>Request" object schema, one property per param. A param
// referencing an entity/message (p.Ref set) renders as a $ref to that
// type's own component schema (registered on d), not an inline scalar
// schema -- see docBuilder.paramSchema.
func (d *docBuilder) renderRequestSchema(params []*ir.Param) (*schemaObject, error) {
	obj := &schemaObject{
		Type:       "object",
		Properties: make(map[string]*schemaObject, len(params)),
	}

	for _, p := range params {
		schema, err := d.paramSchema(p)
		if err != nil {
			return nil, err
		}
		obj.Properties[p.Name] = schema
	}

	return obj, nil
}

// renderPaginatedResponseSchema renders a paginated operation's response
// shape: {type: object, properties: {items: {type: array, items: $ref to
// itemSchemaName}, next_cursor: {type: string}}}, mirroring the object/array
// schema-building pattern already used by renderEntitySchema/
// renderRequestSchema in this file rather than introducing a new shape.
func renderPaginatedResponseSchema(itemSchemaName string) *schemaObject {
	return &schemaObject{
		Type: "object",
		Properties: map[string]*schemaObject{
			"items": {
				Type:  "array",
				Items: &schemaObject{Ref: "#/components/schemas/" + itemSchemaName},
			},
			"next_cursor": {Type: "string"},
		},
	}
}
