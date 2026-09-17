package gogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// TestBackendName locks the backend identifier.
func TestBackendName(t *testing.T) {
	if got := New().Name(); got != "gogen" {
		t.Fatalf("Name() = %q, want gogen", got)
	}
}

// TestGenerateWrapsModuleRenderError proves Generate fails closed with the
// module name when emission is unformattable. Built directly against ir
// types: no parseable .zen source can produce an identifier that breaks
// go/format, so only raw IR reaches this shape.
func TestGenerateWrapsModuleRenderError(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Order",
		Fields: []*ir.Field{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true}},
	}
	module := &ir.Module{Name: "billing", Entities: []*ir.Entity{entity}}
	entity.Module = module

	op := &ir.Operation{
		Name:       "GetOrder",
		Returns:    &ir.TypeRef{Entity: entity},
		Params:     []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}},
		Transports: []ir.Transport{ir.GRPCTransport{}},
	}
	svc := &ir.Service{Name: "not a name", Module: module, Operations: []*ir.Operation{op}}
	module.Services = []*ir.Service{svc}

	_, err := New().Generate(&ir.Schema{Modules: []*ir.Module{module}})
	if err == nil {
		t.Fatalf("Generate must fail on an unformattable model")
	}

	if !strings.Contains(err.Error(), "gogen: module billing:") {
		t.Fatalf("Generate error must name the module, got: %v", err)
	}
}

// TestPBFieldNameCoversNonLowercase covers pbFieldName/toUpperASCII past the
// common snake_case shape: leading capitals and digits pass through the
// non-lowercase branch unchanged.
func TestPBFieldNameCoversNonLowercase(t *testing.T) {
	if got := pbFieldName("user_id"); got != "UserId" {
		t.Fatalf("pbFieldName(user_id) = %q, want UserId", got)
	}

	if got := pbFieldName("field2name"); got != "Field2name" {
		t.Fatalf("pbFieldName(field2name) = %q, want Field2name", got)
	}

	if got := pbFieldName("ID"); got != "ID" {
		t.Fatalf("pbFieldName(ID) = %q, want ID", got)
	}
}

// TestPBGoParamTypeFallback covers the unknown-scalar fallback: an "any"
// Go type with no import, so emission never references a missing symbol.
// (The zero FieldType is TUUID, not unknown -- only an out-of-range scalar
// reaches the fallback.)
func TestPBGoParamTypeFallback(t *testing.T) {
	goType, imp := pbGoParamType(ir.FieldType{Scalar: ir.ScalarType(255)})
	if goType != "any" || imp != "" {
		t.Fatalf("pbGoParamType(unknown) = (%q, %q), want (any, \"\")", goType, imp)
	}
}

// TestNewDefaultWithNamedModule covers pbGoPackage's legacy named-module
// formula under New(): "<root>/zengo/<module>".
func TestNewDefaultWithNamedModule(t *testing.T) {
	file := compileSchema(t, `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/users/{id}"
		auth: required
	}
}`)

	schema, diags := resolver.ResolveWithSchemaDir([]*ast.File{{Name: "schema/v1/iam/user.zen", Decls: file.Decls}}, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["iam/types.go"])
	if !strings.Contains(types, `pb "github.com/zenta-dev/zever/gen/zengo/iam"`) {
		t.Fatalf("New() named module must use the legacy /zengo/ formula, got:\n%s", types)
	}
}

// TestEntityRefParamResolvesFields covers newParamModel's entity-ref arm:
// an entity-typed param alongside a scalar one keeps nested field access.
func TestEntityRefParamResolvesFields(t *testing.T) {
	file := compileSchema(t, `entity User {
	id: uuid @primary
	name: string @validate(min_len: 2)
}

service UserService {
	rpc Rename(id: uuid, user: User) -> User {
		auth: required
	}
}`)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/grpc.go"])

	if !strings.Contains(got, "if req.User == nil {") {
		t.Fatalf("expected nested nil guard req.User, got:\n%s", got)
	}

	if !strings.Contains(got, "if len(req.User.Name) < 2 {") {
		t.Fatalf("expected nested field access req.User.Name, got:\n%s", got)
	}
}

// TestResourceIDNonIDPathParam covers resourceIDFromRequestExpr's fallback:
// a permission-guarded operation whose path id lives under a non-"id" name
// resolves to an empty extractor (still evaluated, never skipped).
func TestResourceIDNonIDPathParam(t *testing.T) {
	entity := &ir.Entity{
		Name: "Order",
		Fields: []*ir.Field{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true},
			{Name: "user_id", Type: ir.FieldType{Scalar: ir.TUUID}},
		},
	}
	module := &ir.Module{Entities: []*ir.Entity{entity}}
	entity.Module = module

	op := &ir.Operation{
		Name:       "GetOrder",
		Returns:    &ir.TypeRef{Entity: entity},
		Params:     []*ir.Param{{Name: "order_id", Type: ir.FieldType{Scalar: ir.TUUID}}},
		Transports: []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: "GET", Path: "/orders/{order_id}"}},
		Permission: &ir.PermissionCheck{Check: "order.read", Resource: entity},
	}
	svc := &ir.Service{Name: "OrderService", Module: module, Operations: []*ir.Operation{op}}
	module.Services = []*ir.Service{svc}

	out, err := New().Generate(&ir.Schema{Modules: []*ir.Module{module}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	router := string(out["app/router.go"])
	if !strings.Contains(router, `func(*http.Request) string { return "" }`) {
		t.Fatalf("expected empty resource-id extractor for non-id path, got:\n%s", router)
	}
}

// TestRenderValidateFuncGuards covers writeRefParamChecks' defensive guards
// directly: a ref param with no validated fields emits nothing, and a
// referenced field without rules is skipped.
func TestRenderValidateFuncGuards(t *testing.T) {
	op := opModel{
		Name:        "Noop",
		RequestType: "NoopRequest",
		Params: []paramModel{
			{Name: "empty", GoName: "Empty", GoType: "*Empty", IsRef: true, RefTypeName: "Empty"},
			{
				Name:        "part",
				GoName:      "Part",
				GoType:      "*Part",
				IsRef:       true,
				RefTypeName: "Part",
				RefFields:   []refFieldModel{{Name: "note", GoName: "Note"}},
			},
		},
	}

	var b strings.Builder

	renderValidateFunc(&b, op, map[string]bool{})

	want := "// validateNoopRequest checks every @validate rule declared on Noop's params,\n" +
		"// returning the first failing rule (checked in schema declaration order) as\n" +
		"// an apperror.InvalidArgument, or nil once every rule passes.\n" +
		"func validateNoopRequest(req *NoopRequest) error {\n" +
		"\tif req.Part == nil {\n" +
		"\t\treturn apperror.New(apperror.InvalidArgument, \"part: is required\")\n" +
		"\t}\n\n" +
		"\treturn nil\n}\n\n"

	if got := b.String(); got != want {
		t.Fatalf("guard-only validator mismatch, got:\n%s", got)
	}
}

// TestRefParamOnGetBindsNothing covers emitParamBind's ref no-op: an
// entity/message-typed param has no string path/query binding (only JSON
// body decode populates it), so the GET handler binds the scalar param and
// leaves the ref param to its validator.
func TestRefParamOnGetBindsNothing(t *testing.T) {
	file := compileSchema(t, `message FilterInput {
	name: string @validate(min_len: 2)
}

entity User {
	id: uuid @primary
}

service UserService {
	rpc Find(id: uuid, filter: FilterInput) -> User {
		http: GET "/users/{id}"
		auth: required
	}
}`)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	router := string(out["app/router.go"])
	if !strings.Contains(router, "req.Id = raw") {
		t.Fatalf("expected scalar path binding, got:\n%s", router)
	}

	if strings.Contains(router, "req.Filter = ") || strings.Contains(router, "req.Filter=") {
		t.Fatalf("ref param must have no string binding, got:\n%s", router)
	}

	if !strings.Contains(string(out["app/grpc.go"]), "if req.Filter == nil {") {
		t.Fatalf("expected ref nil guard, got:\n%s", out["app/grpc.go"])
	}
}

// TestNumericLiteralDefault covers numericLiteral's fallback for a decoded
// value of unexpected dynamic type.
func TestNumericLiteralDefault(t *testing.T) {
	if got := numericLiteral("7"); got != "7" {
		t.Fatalf("numericLiteral(string) = %q, want 7", got)
	}

	if got := numericLiteral(int64(7)); got != "7" {
		t.Fatalf("numericLiteral(int64) = %q, want 7", got)
	}

	if got := numericLiteral(0.5); got != "0.5" {
		t.Fatalf("numericLiteral(float64) = %q, want 0.5", got)
	}
}
