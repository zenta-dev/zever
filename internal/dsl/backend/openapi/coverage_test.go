// Coverage tests for the openapi backend's enum, method-key, validation,
// and helper branches that the main tests do not exercise.
//
// Reachability notes:
//   - Generate's two json.MarshalIndent error branches (openapi.go) are
//     unreachable via the public API with real inputs: openAPIDoc is a plain
//     struct of strings, maps, slices, and pointers with no MarshalJSON
//     methods and no interface fields that could hold an unmarshalable
//     value, so marshaling cannot fail. They are covered via the
//     jsonMarshalIndent test seam in marshal_error_test.go instead.
//     (Caller-controlled any-typed Validation.Args values only flow through
//     toInt64/toFloat64 into concrete numeric fields, so no fault-injectable
//     input reaches the marshaler; a seam is required. The merged branch in
//     particular cannot be triggered by input alone, since any value failing
//     the merged marshal would already fail its own module marshal first.)
//   - Every other branch below is covered, either via Generate or by direct
//     unit calls into the same package.
package openapi

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// TestGenerateNamedEnumSchema proves a named `enum` declaration renders once
// as a shared {type: string, enum: [...]} component and that fields
// referencing it by name become $refs to it, while inline enum(...) fields
// keep rendering inline.
func TestGenerateNamedEnumSchema(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `enum Status { active, inactive }

	entity Task {
		id: uuid @primary
		status: Status
		inline: enum(x, y)
	}

	service Svc {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/v1/tasks/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)

	content, ok := out["default/openapi.json"]
	if !ok {
		t.Fatalf("missing default/openapi.json, got keys %v", keysOf(out))
	}

	doc := decodeDoc(t, content)
	schemas := componentSchemas(t, doc)

	status, ok := schemas["Status"]
	if !ok {
		t.Fatalf("components.schemas missing Status: %v", keysOfAny(schemas))
	}

	statusObj := asMap(t, status)
	if statusObj["type"] != "string" {
		t.Fatalf("Status schema = %v, want type=string", statusObj)
	}

	enumVals := asSlice(t, statusObj["enum"])
	if len(enumVals) != 2 || enumVals[0] != "active" || enumVals[1] != "inactive" {
		t.Fatalf("Status enum values = %v, want [active inactive]", enumVals)
	}

	task := asMap(t, schemas["Task"])
	props := asMap(t, task["properties"])

	refField := asMap(t, props["status"])
	if refField["$ref"] != "#/components/schemas/Status" {
		t.Fatalf("Task.status schema = %v, want $ref #/components/schemas/Status", refField)
	}

	inline := asMap(t, props["inline"])
	if inline["type"] != "string" {
		t.Fatalf("Task.inline schema = %v, want inline string enum", inline)
	}

	if _, hasRef := inline["$ref"]; hasRef {
		t.Fatalf("inline enum field must not become a $ref: %v", inline)
	}

	// The merged spec qualifies the shared enum with its module label.
	merged := decodeDoc(t, out["openapi.json"])
	mergedSchemas := componentSchemas(t, merged)

	if _, ok := mergedSchemas["default_Status"]; !ok {
		t.Fatalf("merged spec missing default_Status: %v", keysOfAny(mergedSchemas))
	}
}

// TestAddEnumSchema_unknownAndIdempotent proves an unknown enum name maps to
// "" (the defensive resolver-invariant branch) and that registering the same
// enum twice renders it only once.
func TestAddEnumSchema_unknownAndIdempotent(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	b := newDocBuilder("t", bareQualify, map[string]*ir.Enum{})

	if got := b.addEnumSchema("Missing"); got != "" {
		t.Fatalf("addEnumSchema(unknown) = %q, want empty", got)
	}

	b.enums["Status"] = &ir.Enum{Name: "Status", Values: []string{"active"}, Module: mod}

	first := b.addEnumSchema("Status")
	if first != "Status" {
		t.Fatalf("addEnumSchema(Status) = %q, want Status", first)
	}

	second := b.addEnumSchema("Status")
	if second != first {
		t.Fatalf("second addEnumSchema(Status) = %q, want %q (idempotent)", second, first)
	}

	if len(b.doc.Components.Schemas) != 1 {
		t.Fatalf("expected exactly 1 component schema, got %v", b.doc.Components.Schemas)
	}
}

// TestFieldSchema_namedVsInline proves a named enum becomes a $ref while an
// inline enum renders its values directly.
func TestFieldSchema_namedVsInline(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	b := newDocBuilder("t", bareQualify, map[string]*ir.Enum{
		"Status": {Name: "Status", Values: []string{"active"}, Module: mod},
	})

	named := b.fieldSchema(ir.FieldType{Scalar: ir.TEnum, EnumName: "Status"}, nil)
	if named.Ref != "#/components/schemas/Status" {
		t.Fatalf("named enum fieldSchema = %+v, want $ref to Status", named)
	}

	inline := b.fieldSchema(ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"x"}}, nil)
	if inline.Ref != "" || inline.Type != "string" || len(inline.Enum) != 1 {
		t.Fatalf("inline enum fieldSchema = %+v, want inline string enum", inline)
	}
}

// TestAddTypeRefSchema_branches covers nil, entity, message, and empty refs.
func TestAddTypeRefSchema_branches(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	entity := &ir.Entity{Name: "Widget", Module: mod}
	message := &ir.Message{Name: "Payload", Module: mod}

	b := newDocBuilder("t", bareQualify, map[string]*ir.Enum{})

	if got := b.addTypeRefSchema(nil); got != "" {
		t.Fatalf("addTypeRefSchema(nil) = %q, want empty", got)
	}

	if got := b.addTypeRefSchema(&ir.TypeRef{}); got != "" {
		t.Fatalf("addTypeRefSchema(empty) = %q, want empty", got)
	}

	if got := b.addTypeRefSchema(&ir.TypeRef{Entity: entity}); got != "Widget" {
		t.Fatalf("addTypeRefSchema(entity) = %q, want Widget", got)
	}

	if got := b.addTypeRefSchema(&ir.TypeRef{Message: message}); got != "Payload" {
		t.Fatalf("addTypeRefSchema(message) = %q, want Payload", got)
	}
}

// TestParamSchema_branches proves a ref param renders as a $ref (registering
// the referenced schema) while a scalar param renders inline.
func TestParamSchema_branches(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	entity := &ir.Entity{Name: "Widget", Module: mod}
	b := newDocBuilder("t", bareQualify, map[string]*ir.Enum{})

	ref := b.paramSchema(&ir.Param{Name: "w", Ref: &ir.TypeRef{Entity: entity}})
	if ref.Ref != "#/components/schemas/Widget" {
		t.Fatalf("ref paramSchema = %+v, want $ref to Widget", ref)
	}

	scalar := b.paramSchema(&ir.Param{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}})
	if scalar.Type != "string" || scalar.Format != "uuid" {
		t.Fatalf("scalar paramSchema = %+v, want string/uuid", scalar)
	}
}

// TestHTTPMethodKey_table covers every method branch plus the passthrough
// default for methods outside the five known verbs.
func TestHTTPMethodKey_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		want   string
	}{
		{name: "GET", method: "GET", want: "get"},
		{name: "POST", method: "POST", want: "post"},
		{name: "PUT", method: "PUT", want: "put"},
		{name: "PATCH", method: "PATCH", want: "patch"},
		{name: "DELETE", method: "DELETE", want: "delete"},
		{name: "unknown", method: "OPTIONS", want: "OPTIONS"},
		{name: "lowercase", method: "get", want: "get"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := httpMethodKey(tt.method); got != tt.want {
				t.Fatalf("httpMethodKey(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

// TestGenerateModulePathConflict proves two RPCs in the same module
// declaring the identical method+path fail Generate with an error naming
// both owners, covering the per-module populateDoc error branch.
func TestGenerateModulePathConflict(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `entity Widget {
		id: uuid @primary
	}

	entity Gadget {
		id: uuid @primary
	}

	service A {
		rpc GetWidget(id: uuid) -> Widget {
			http: GET "/widgets/{id}"
			auth: required
		}
	}

	service B {
		rpc GetGadget(id: uuid) -> Gadget {
			http: GET "/widgets/{id}"
			auth: required
		}
	}`)

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate: expected a path-conflict error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "A.GetWidget") || !strings.Contains(msg, "B.GetGadget") {
		t.Fatalf("error %q should name both conflicting RPCs", msg)
	}
}

// TestGeneratePutPatchDeleteMethods proves PUT/PATCH render a requestBody
// while DELETE renders extra params as query parameters, covering the
// httpMethodKey PUT/PATCH/DELETE branches end to end.
func TestGeneratePutPatchDeleteMethods(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `entity Order {
		id: uuid @primary
		amount_cents: int64
	}

	service OrderService {
		rpc PutOrder(id: uuid, amount_cents: int64) -> Order {
			http: PUT "/v1/orders/{id}"
			auth: required
		}

		rpc PatchOrder(id: uuid, amount_cents: int64) -> Order {
			http: PATCH "/v1/orders/{id}"
			auth: required
		}

		rpc DeleteOrder(id: uuid, reason: string) -> Order {
			http: DELETE "/v1/orders/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])

	put := operationAt(t, doc, "/v1/orders/{id}", "put")
	if _, hasBody := put["requestBody"]; !hasBody {
		t.Fatalf("PUT operation should have a requestBody: %v", put)
	}

	patch := operationAt(t, doc, "/v1/orders/{id}", "patch")
	if _, hasBody := patch["requestBody"]; !hasBody {
		t.Fatalf("PATCH operation should have a requestBody: %v", patch)
	}

	del := operationAt(t, doc, "/v1/orders/{id}", "delete")
	if _, hasBody := del["requestBody"]; hasBody {
		t.Fatalf("DELETE operation should not have a requestBody: %v", del)
	}

	params := asSlice(t, del["parameters"])

	var haveQuery bool

	for _, p := range params {
		if pm := asMap(t, p); pm["in"] == "query" && pm["name"] == "reason" {
			haveQuery = true
		}
	}

	if !haveQuery {
		t.Fatalf("DELETE operation missing query param reason: %v", params)
	}
}

// TestGenerateValidationGteLtLteAndURLFormat proves the remaining validation
// branches end to end: gte/lt/lte bounds, and format "url" mapping to the
// standard "uri" name.
func TestGenerateValidationGteLtLteAndURLFormat(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `entity Item {
		id: uuid @primary
		link: string @validate(format: "url")
		stock: int32 @validate(gte: 0, lte: 100)
		rating: float64 @validate(lt: 5.5)
	}

	service Svc {
		rpc GetItem(id: uuid) -> Item {
			http: GET "/v1/items/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	schemas := componentSchemas(t, doc)
	props := asMap(t, asMap(t, schemas["Item"])["properties"])

	link := asMap(t, props["link"])
	if link["format"] != "uri" {
		t.Fatalf("link schema = %v, want format=uri", link)
	}

	stock := asMap(t, props["stock"])
	if stock["minimum"] != float64(0) || stock["maximum"] != float64(100) {
		t.Fatalf("stock schema = %v, want minimum=0 maximum=100", stock)
	}

	if _, hasExclusive := stock["exclusiveMinimum"]; hasExclusive {
		t.Fatalf("gte must not set exclusiveMinimum: %v", stock)
	}

	if _, hasExclusive := stock["exclusiveMaximum"]; hasExclusive {
		t.Fatalf("lte must not set exclusiveMaximum: %v", stock)
	}

	rating := asMap(t, props["rating"])
	if rating["maximum"] != float64(5.5) || rating["exclusiveMaximum"] != true {
		t.Fatalf("rating schema = %v, want maximum=5.5 exclusiveMaximum=true", rating)
	}
}

// TestApplyValidation_table covers the branches no DSL spelling reaches:
// non-string format values, unknown kinds, float64 min_len, and string
// max_len (the resolver only ever emits int64 numerics).
func TestApplyValidation_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rules []ir.Validation
		check func(t *testing.T, s *schemaObject)
	}{
		{
			name:  "format non-string ignored",
			rules: []ir.Validation{{Kind: "format", Args: map[string]any{"value": int64(7)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Format != "" {
					t.Fatalf("format with non-string value set Format=%q", s.Format)
				}
			},
		},
		{
			name:  "unknown kind ignored",
			rules: []ir.Validation{{Kind: "bogus", Args: map[string]any{"value": int64(1)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Format != "" || s.MinLength != nil || s.Minimum != nil {
					t.Fatalf("unknown kind mutated the schema: %+v", s)
				}
			},
		},
		{
			name:  "min_len float64",
			rules: []ir.Validation{{Kind: "min_len", Args: map[string]any{"value": float64(3)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.MinLength == nil || *s.MinLength != 3 {
					t.Fatalf("MinLength = %v, want 3", s.MinLength)
				}
			},
		},
		{
			name:  "max_len non-numeric",
			rules: []ir.Validation{{Kind: "max_len", Args: map[string]any{"value": "many"}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.MaxLength == nil || *s.MaxLength != 0 {
					t.Fatalf("MaxLength = %v, want 0 (defensive default)", s.MaxLength)
				}
			},
		},
		{
			name:  "gt int64",
			rules: []ir.Validation{{Kind: "gt", Args: map[string]any{"value": int64(2)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Minimum == nil || *s.Minimum != 2 || !s.ExclusiveMinimum {
					t.Fatalf("gt schema = %+v, want minimum=2 exclusive", s)
				}
			},
		},
		{
			name:  "gte float64",
			rules: []ir.Validation{{Kind: "gte", Args: map[string]any{"value": float64(1.5)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Minimum == nil || *s.Minimum != 1.5 || s.ExclusiveMinimum {
					t.Fatalf("gte schema = %+v, want minimum=1.5 inclusive", s)
				}
			},
		},
		{
			name:  "lt string default",
			rules: []ir.Validation{{Kind: "lt", Args: map[string]any{"value": "x"}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Maximum == nil || *s.Maximum != 0 || !s.ExclusiveMaximum {
					t.Fatalf("lt schema = %+v, want maximum=0 exclusive", s)
				}
			},
		},
		{
			name:  "lte int64",
			rules: []ir.Validation{{Kind: "lte", Args: map[string]any{"value": int64(9)}}},
			check: func(t *testing.T, s *schemaObject) {
				t.Helper()
				if s.Maximum == nil || *s.Maximum != 9 || s.ExclusiveMaximum {
					t.Fatalf("lte schema = %+v, want maximum=9 inclusive", s)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &schemaObject{Type: "string"}
			applyValidation(s, tt.rules)
			tt.check(t, s)
		})
	}
}

// TestOpenAPIFormatName_table proves the url->uri mapping and passthrough.
func TestOpenAPIFormatName_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		want   string
	}{
		{format: "url", want: "uri"},
		{format: "email", want: "email"},
		{format: "uuid", want: "uuid"},
		{format: "hostname", want: "hostname"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			if got := openAPIFormatName(tt.format); got != tt.want {
				t.Fatalf("openAPIFormatName(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

// TestToInt64_table covers int64, float64, and the defensive default.
func TestToInt64_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  int64
	}{
		{name: "int64", value: int64(7), want: 7},
		{name: "float64", value: float64(7.9), want: 7},
		{name: "other", value: "7", want: 0},
		{name: "nil", value: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := toInt64(tt.value); got != tt.want {
				t.Fatalf("toInt64(%v) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

// TestToFloat64_table covers int64, float64, and the defensive default.
func TestToFloat64_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  float64
	}{
		{name: "int64", value: int64(7), want: 7},
		{name: "float64", value: float64(7.5), want: 7.5},
		{name: "other", value: "7", want: 0},
		{name: "nil", value: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := toFloat64(tt.value); got != tt.want {
				t.Fatalf("toFloat64(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestScalarOpenAPI_invalid proves the trailing defensive return for values
// outside the ir.ScalarType vocabulary.
func TestScalarOpenAPI_invalid(t *testing.T) {
	t.Parallel()

	typ, format := scalarOpenAPI(ir.ScalarType(99))
	if typ != "" || format != "" {
		t.Fatalf("scalarOpenAPI(invalid) = (%q, %q), want empty", typ, format)
	}
}
