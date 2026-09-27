package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// compileSchema runs a .zen source string through the parser and resolver,
// failing the test on any diagnostic from either stage, matching the proto
// backend's own compileSchema test helper.
func compileSchema(t *testing.T, src string) *ir.Schema {
	t.Helper()

	p := parser.New("test.zen", []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

func compileMultiModule(t *testing.T, files map[string]string) *ir.Schema {
	t.Helper()

	astFiles := make([]*ast.File, 0, len(files))
	for name, src := range files {
		p := parser.New(name, []byte(src))
		f, diags := p.ParseFile()
		if diags.HasErrors() {
			t.Fatalf("parse errors in %s: %v", name, diags)
		}
		astFiles = append(astFiles, f)
	}
	schema, diags := resolver.ResolveWithSchemaDir(astFiles, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}
	return schema
}

func mustGenerate(t *testing.T, schema *ir.Schema) map[string][]byte {
	t.Helper()

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return out
}

// decodeDoc unmarshals one generated spec into a generic map for assertions.
func decodeDoc(t *testing.T, content []byte) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal spec: %v\n%s", err, content)
	}

	return doc
}

// asMap asserts v decoded to a JSON object, failing the test (not panicking)
// if it didn't.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object, got %T: %v", v, v)
	}

	return m
}

// asSlice asserts v decoded to a JSON array, failing the test if it didn't.
func asSlice(t *testing.T, v any) []any {
	t.Helper()

	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected a JSON array, got %T: %v", v, v)
	}

	return s
}

// operationAt navigates doc.paths[path][method], failing the test if either
// the path or the method-keyed operation is missing.
func operationAt(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()

	paths := asMap(t, doc["paths"])

	pi, ok := paths[path]
	if !ok {
		t.Fatalf("paths missing %q, got %v", path, keysOfAny(paths))
	}

	pim := asMap(t, pi)

	op, ok := pim[method]
	if !ok {
		t.Fatalf("path item %q missing %q operation, got %v", path, method, keysOfAny(pim))
	}

	return asMap(t, op)
}

// componentSchemas navigates doc.components.schemas.
func componentSchemas(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()

	return asMap(t, asMap(t, doc["components"])["schemas"])
}

// refOf navigates a requestBody or response object's
// content["application/json"].schema.$ref.
func refOf(t *testing.T, container map[string]any, key string) any {
	t.Helper()

	body := asMap(t, container[key])
	content := asMap(t, body["content"])
	media := asMap(t, content["application/json"])
	schema := asMap(t, media["schema"])

	return schema["$ref"]
}

// responseSchemaOf navigates a requestBody or response object's
// content["application/json"].schema, returning the raw schema object
// (unlike refOf, which assumes it's a bare $ref).
func responseSchemaOf(t *testing.T, container map[string]any, key string) map[string]any {
	t.Helper()

	body := asMap(t, container[key])
	content := asMap(t, body["content"])
	media := asMap(t, content["application/json"])

	return asMap(t, media["schema"])
}

// TestPaginatedOperationResponseSchema proves a `paginated: true` operation's
// 200 response schema is wrapped as {type: object, properties: {items:
// {type: array, items: $ref}, next_cursor: {type: string}}} instead of a
// bare $ref to the entity, and that it additionally gets optional
// cursor/limit query parameters.
func TestPaginatedOperationResponseSchema(t *testing.T) {
	schema := compileSchema(t, `entity Task {
		id: uuid @primary
		title: string
	}

	service TaskService {
		rpc ListTasks(user_id: uuid) -> Task {
			http: GET "/v1/tasks"
			paginated: true
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/tasks", "get")

	responses := asMap(t, op["responses"])
	respSchema := responseSchemaOf(t, responses, "200")

	if respSchema["type"] != "object" {
		t.Fatalf("paginated response schema type = %v, want object: %v", respSchema["type"], respSchema)
	}

	props := asMap(t, respSchema["properties"])

	items := asMap(t, props["items"])
	if items["type"] != "array" {
		t.Fatalf("items property type = %v, want array: %v", items["type"], items)
	}

	itemsRef := asMap(t, items["items"])["$ref"]
	if itemsRef != "#/components/schemas/Task" {
		t.Fatalf("items[].$ref = %v, want #/components/schemas/Task", itemsRef)
	}

	nextCursor := asMap(t, props["next_cursor"])
	if nextCursor["type"] != "string" {
		t.Fatalf("next_cursor type = %v, want string: %v", nextCursor["type"], nextCursor)
	}

	var haveCursor, haveLimit bool

	for _, p := range asSlice(t, op["parameters"]) {
		pm := asMap(t, p)
		if pm["in"] != "query" {
			continue
		}

		switch pm["name"] {
		case "cursor":
			haveCursor = true

			if pm["required"] != false {
				t.Fatalf("cursor param required = %v, want false", pm["required"])
			}
		case "limit":
			haveLimit = true
		}
	}

	if !haveCursor || !haveLimit {
		t.Fatalf("expected cursor and limit query params, got %v", op["parameters"])
	}
}

// TestNonPaginatedOperationResponseSchemaUnaffected proves a plain
// (non-paginated) operation's response is still a bare $ref, and it gets no
// cursor/limit query parameters -- the regression check for this feature.
func TestNonPaginatedOperationResponseSchemaUnaffected(t *testing.T) {
	schema := compileSchema(t, `entity Task {
		id: uuid @primary
		title: string
	}

	service TaskService {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/v1/tasks/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/tasks/{id}", "get")

	responses := asMap(t, op["responses"])

	ref := refOf(t, responses, "200")
	if ref != "#/components/schemas/Task" {
		t.Fatalf("200 response $ref = %v, want #/components/schemas/Task", ref)
	}

	for _, p := range asSlice(t, op["parameters"]) {
		pm := asMap(t, p)
		if pm["name"] == "cursor" || pm["name"] == "limit" {
			t.Fatalf("non-paginated operation must not get a %v query param", pm["name"])
		}
	}
}

func TestName(t *testing.T) {
	if got := New().Name(); got != "openapi" {
		t.Fatalf("Name() = %q, want %q", got, "openapi")
	}
}

// TestGetRPCWithPathPlaceholder covers a GET RPC with a single path
// placeholder and no other params: per-module spec has the path, no
// requestBody, one path parameter, and a 200 response $ref-ing the correct
// entity schema.
func TestGetRPCWithPathPlaceholder(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
		amount_cents: int64
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)

	content, ok := out["default/openapi.json"]
	if !ok {
		t.Fatalf("missing default/openapi.json, got keys %v", keysOf(out))
	}

	doc := decodeDoc(t, content)
	op := operationAt(t, doc, "/v1/orders/{id}", "get")

	if _, hasBody := op["requestBody"]; hasBody {
		t.Fatalf("GET operation should not have a requestBody: %v", op)
	}

	params := asSlice(t, op["parameters"])
	if len(params) != 1 {
		t.Fatalf("expected exactly 1 parameter, got %v", params)
	}

	p := asMap(t, params[0])
	if p["in"] != "path" || p["name"] != "id" {
		t.Fatalf("expected path param %q, got %v", "id", p)
	}

	responses := asMap(t, op["responses"])

	if _, ok := responses["200"]; !ok {
		t.Fatalf("missing 200 response: %v", responses)
	}

	ref := refOf(t, responses, "200")
	if ref != "#/components/schemas/Order" {
		t.Fatalf("200 response $ref = %v, want #/components/schemas/Order", ref)
	}

	schemas := componentSchemas(t, doc)
	if _, ok := schemas["Order"]; !ok {
		t.Fatalf("components.schemas missing Order: %v", schemas)
	}
}

// TestOptionalFieldExcludedFromRequiredAndNullable proves a field declared
// optional (`field: type?`) is excluded from the entity schema's `required`
// list and marked `nullable: true`, while a non-optional @primary field
// stays required and non-nullable.
func TestOptionalFieldExcludedFromRequiredAndNullable(t *testing.T) {
	schema := compileSchema(t, `entity User {
		id: uuid @primary
		nickname: string?
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/v1/users/{id}"
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

	user := asMap(t, schemas["User"])
	props := asMap(t, user["properties"])

	nickname := asMap(t, props["nickname"])
	if nickname["nullable"] != true {
		t.Fatalf("nickname schema = %v, want nullable=true", nickname)
	}

	id := asMap(t, props["id"])
	if _, hasNullable := id["nullable"]; hasNullable {
		t.Fatalf("id schema should not carry nullable: %v", id)
	}

	required := asSlice(t, user["required"])
	for _, r := range required {
		if r == "nickname" {
			t.Fatalf("required list should not include optional field nickname: %v", required)
		}
	}

	foundID := false

	for _, r := range required {
		if r == "id" {
			foundID = true
		}
	}

	if !foundID {
		t.Fatalf("required list should include primary field id: %v", required)
	}
}

// TestValidateRuleRendersJSONSchemaConstraint proves an entity field's and
// an RPC param's @validate rule each render as the matching JSON Schema
// constraint keyword: format (with "url" mapped to the standard "uri" name),
// minLength/maxLength, and minimum/maximum (+ exclusiveMinimum for gt),
// following the same per-field schema-object pattern
// TestOptionalFieldExcludedFromRequiredAndNullable already exercises for
// optional/nullable.
func TestValidateRuleRendersJSONSchemaConstraint(t *testing.T) {
	schema := compileSchema(t, `entity User {
		id: uuid @primary
		email: string @validate(format: "email")
		bio: string @validate(min_len: 1, max_len: 280)
	}

	service UserService {
		rpc CreateUser(email: string @validate(format: "email"), age: int32 @validate(gt: 17)) -> User {
			http: POST "/v1/users"
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

	user := asMap(t, schemas["User"])
	props := asMap(t, user["properties"])

	email := asMap(t, props["email"])
	if email["format"] != "email" {
		t.Fatalf("email schema = %v, want format=email", email)
	}

	bio := asMap(t, props["bio"])
	if bio["minLength"] != float64(1) || bio["maxLength"] != float64(280) {
		t.Fatalf("bio schema = %v, want minLength=1 maxLength=280", bio)
	}

	req := asMap(t, schemas["UserServiceCreateUserRequest"])
	reqProps := asMap(t, req["properties"])

	reqEmail := asMap(t, reqProps["email"])
	if reqEmail["format"] != "email" {
		t.Fatalf("UserServiceCreateUserRequest.email schema = %v, want format=email", reqEmail)
	}

	age := asMap(t, reqProps["age"])
	if age["minimum"] != float64(17) {
		t.Fatalf("UserServiceCreateUserRequest.age schema = %v, want minimum=17", age)
	}

	if age["exclusiveMinimum"] != true {
		t.Fatalf("UserServiceCreateUserRequest.age schema = %v, want exclusiveMinimum=true (gt is exclusive)", age)
	}
}

// TestPostRPCPathAndExtraParams proves a path param never doubles up in the
// synthesized request schema: it appears only in parameters, and the
// remaining params appear only in the request schema.
// TestSameRPCNameDifferentServicesDoesNotCollide covers the fix for
// addRequestSchema previously namespacing a synthesized request schema by
// rpc.Name + module only (not svc.Name, unlike OperationID's own
// "<Service>_<RPCName>" scheme): two services in the same module both
// declaring a same-named RPC with body params used to silently produce ONE
// component schema (whichever was processed last), leaving the other
// operation's $ref pointing at the wrong (or even self-referencing, as
// found regenerating examples/showcase's real output) schema with zero
// diagnostic. Both request schemas must now exist, distinctly named and
// correctly content-matched to their own service.
func TestSameRPCNameDifferentServicesDoesNotCollide(t *testing.T) {
	schema := compileSchema(t, `entity Widget {
		id: uuid @primary
	}

	entity Gadget {
		id: uuid @primary
	}

	service WidgetService {
		rpc Create(name: string) -> Widget {
			http: POST "/v1/widgets"
			auth: required
		}
	}

	service GadgetService {
		rpc Create(label: string) -> Gadget {
			http: POST "/v1/gadgets"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	schemas := componentSchemas(t, doc)

	widgetReq, ok := schemas["WidgetServiceCreateRequest"]
	if !ok {
		t.Fatalf("missing WidgetServiceCreateRequest component schema: %v", keysOfAny(schemas))
	}

	gadgetReq, ok := schemas["GadgetServiceCreateRequest"]
	if !ok {
		t.Fatalf("missing GadgetServiceCreateRequest component schema: %v", keysOfAny(schemas))
	}

	widgetProps := asMap(t, asMap(t, widgetReq)["properties"])
	if _, ok := widgetProps["name"]; !ok {
		t.Fatalf("WidgetServiceCreateRequest missing its own %q param: %v", "name", widgetProps)
	}
	if _, ok := widgetProps["label"]; ok {
		t.Fatalf("WidgetServiceCreateRequest wrongly carries GadgetService's %q param: %v", "label", widgetProps)
	}

	gadgetProps := asMap(t, asMap(t, gadgetReq)["properties"])
	if _, ok := gadgetProps["label"]; !ok {
		t.Fatalf("GadgetServiceCreateRequest missing its own %q param: %v", "label", gadgetProps)
	}
	if _, ok := gadgetProps["name"]; ok {
		t.Fatalf("GadgetServiceCreateRequest wrongly carries WidgetService's %q param: %v", "name", gadgetProps)
	}

	widgetOp := operationAt(t, doc, "/v1/widgets", "post")
	if refOf(t, widgetOp, "requestBody") != "#/components/schemas/WidgetServiceCreateRequest" {
		t.Fatalf("widget op requestBody $ref = %v, want WidgetServiceCreateRequest", refOf(t, widgetOp, "requestBody"))
	}

	gadgetOp := operationAt(t, doc, "/v1/gadgets", "post")
	if refOf(t, gadgetOp, "requestBody") != "#/components/schemas/GadgetServiceCreateRequest" {
		t.Fatalf("gadget op requestBody $ref = %v, want GadgetServiceCreateRequest", refOf(t, gadgetOp, "requestBody"))
	}
}

func TestPostRPCPathAndExtraParams(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
		user_id: uuid
		amount_cents: int64
	}

	service OrderService {
		rpc UpdateOrder(id: uuid, amount_cents: int64) -> Order {
			http: POST "/v1/orders/{id}"
			auth: required
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/orders/{id}", "post")

	params := asSlice(t, op["parameters"])
	if len(params) != 1 {
		t.Fatalf("expected exactly 1 path parameter, got %v", params)
	}

	p := asMap(t, params[0])
	if p["in"] != "path" || p["name"] != "id" {
		t.Fatalf("expected path param %q, got %v", "id", p)
	}

	if _, hasBody := op["requestBody"]; !hasBody {
		t.Fatalf("expected requestBody on POST operation: %v", op)
	}

	ref := refOf(t, op, "requestBody")
	if ref != "#/components/schemas/OrderServiceUpdateOrderRequest" {
		t.Fatalf("requestBody $ref = %v, want #/components/schemas/OrderServiceUpdateOrderRequest", ref)
	}

	schemas := componentSchemas(t, doc)

	reqSchema, ok := schemas["OrderServiceUpdateOrderRequest"]
	if !ok {
		t.Fatalf("missing OrderServiceUpdateOrderRequest component schema: %v", schemas)
	}

	props := asMap(t, asMap(t, reqSchema)["properties"])
	if _, ok := props["id"]; ok {
		t.Fatalf("path param %q must not be duplicated in the request schema: %v", "id", props)
	}

	if _, ok := props["amount_cents"]; !ok {
		t.Fatalf("request schema missing non-path param %q: %v", "amount_cents", props)
	}

	if len(props) != 1 {
		t.Fatalf("request schema should have exactly 1 property, got %v", props)
	}
}

// TestAuthRolesMapping covers auth: required(roles: {admin}).
func TestAuthRolesMapping(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required(roles: {admin})
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/orders/{id}", "get")

	security := asSlice(t, op["security"])
	if len(security) != 1 {
		t.Fatalf("expected exactly one security requirement, got %v", op["security"])
	}

	sec0 := asMap(t, security[0])
	if _, ok := sec0["bearerAuth"]; !ok {
		t.Fatalf("expected bearerAuth security requirement, got %v", sec0)
	}

	roles := asSlice(t, op["x-roles"])
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("x-roles = %v, want [admin]", op["x-roles"])
	}
}

// TestPermissionMapping covers permission: check(...).
func TestPermissionMapping(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
		user_id: uuid
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			permission: check("owns_order", resource: Order, owner_field: user_id)
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/orders/{id}", "get")

	xperm := asMap(t, op["x-permission"])

	if xperm["check"] != "owns_order" {
		t.Fatalf("x-permission.check = %v, want owns_order", xperm["check"])
	}

	if xperm["resource"] != "Order" {
		t.Fatalf("x-permission.resource = %v, want Order", xperm["resource"])
	}

	if xperm["owner_field"] != "user_id" {
		t.Fatalf("x-permission.owner_field = %v, want user_id", xperm["owner_field"])
	}
}

// TestErrorResponsesMapping proves errors: {...} adds one response per
// distinct HTTP status alongside the existing 200 response, with the
// expected description for a single, non-colliding code and for a code
// with a message.
func TestErrorResponsesMapping(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required
			errors: { not_found("no such order"), permission_denied }
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/orders/{id}", "get")

	responses := asMap(t, op["responses"])

	if _, ok := responses["200"]; !ok {
		t.Fatalf("missing 200 response alongside error responses: %v", responses)
	}

	notFound := asMap(t, responses["404"])
	if notFound["description"] != "NOT_FOUND: no such order" {
		t.Fatalf("404 description = %v, want %q", notFound["description"], "NOT_FOUND: no such order")
	}

	denied := asMap(t, responses["403"])
	if denied["description"] != "PERMISSION_DENIED" {
		t.Fatalf("403 description = %v, want %q", denied["description"], "PERMISSION_DENIED")
	}
}

// TestErrorResponsesStatusCollisionMerge is the dedicated collision-merge
// test: an rpc declaring two error codes that share the same HTTP status
// (invalid_argument and out_of_range both map to 400, per Google's own
// canonical error-code-to-status table) must produce exactly ONE "400"
// response whose description contains both codes, in declaration order —
// proving the merge path, not just the non-colliding path above.
func TestErrorResponsesStatusCollisionMerge(t *testing.T) {
	schema := compileSchema(t, `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required
			errors: { invalid_argument, out_of_range("index too large") }
		}
	}`)

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["default/openapi.json"])
	op := operationAt(t, doc, "/v1/orders/{id}", "get")

	responses := asMap(t, op["responses"])

	// Exactly one "400" entry — not overwritten by the second colliding
	// code, and no stray second key either.
	resp400 := asMap(t, responses["400"])

	wantDesc := "INVALID_ARGUMENT; OUT_OF_RANGE: index too large"
	if resp400["description"] != wantDesc {
		t.Fatalf("400 description = %v, want %q (both colliding codes merged, declaration order)",
			resp400["description"], wantDesc)
	}

	// 200 must still be present alongside the merged error response.
	if _, ok := responses["200"]; !ok {
		t.Fatalf("missing 200 response alongside merged 400: %v", responses)
	}

	if len(responses) != 2 {
		t.Fatalf("expected exactly 2 responses (200, merged 400), got %d: %v", len(responses), responses)
	}
}

// TestMergedSpecQualifiesSameNameEntities proves two modules each declaring
// a bare-named "Widget" entity end up under distinct module-qualified
// component names in the merged spec, with no collision/overwrite. Two
// real modules can never actually declare the same entity name through the
// parser/resolver (names are global across modules by design — see
// resolver_symbols.go), so this scenario is exercised via a hand-built
// *ir.Schema instead, the same established pattern the atlas backend's own
// TestGenerateTableNameCollision uses for an IR-level edge case that can't
// be authored as real .zen source.
func TestMergedSpecQualifiesSameNameEntities(t *testing.T) {
	moduleA := &ir.Module{Name: "ModuleA"}
	widgetA := &ir.Entity{
		Name:   "Widget",
		Module: moduleA,
		Fields: []*ir.Field{{Name: "id", Primary: true, Type: ir.FieldType{Scalar: ir.TUUID}}, {Name: "a_field", Type: ir.FieldType{Scalar: ir.TString}}},
	}
	moduleA.Entities = []*ir.Entity{widgetA}
	moduleA.Services = []*ir.Service{{
		Name:   "ModuleAService",
		Module: moduleA,
		Operations: []*ir.Operation{{
			Name:       "GetWidget",
			Params:     []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}},
			Returns:    &ir.TypeRef{Entity: widgetA},
			Transports: []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: "GET", Path: "/a/widgets/{id}"}},
		}},
	}}

	moduleB := &ir.Module{Name: "ModuleB"}
	widgetB := &ir.Entity{
		Name:   "Widget",
		Module: moduleB,
		Fields: []*ir.Field{{Name: "id", Primary: true, Type: ir.FieldType{Scalar: ir.TUUID}}, {Name: "b_field", Type: ir.FieldType{Scalar: ir.TString}}},
	}
	moduleB.Entities = []*ir.Entity{widgetB}
	moduleB.Services = []*ir.Service{{
		Name:   "ModuleBService",
		Module: moduleB,
		Operations: []*ir.Operation{{
			Name:       "GetWidget",
			Params:     []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}},
			Returns:    &ir.TypeRef{Entity: widgetB},
			Transports: []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: "GET", Path: "/b/widgets/{id}"}},
		}},
	}}

	schema := &ir.Schema{Modules: []*ir.Module{moduleA, moduleB}}

	out := mustGenerate(t, schema)
	doc := decodeDoc(t, out["openapi.json"])

	schemas := componentSchemas(t, doc)

	aWidget, ok := schemas["ModuleA_Widget"]
	if !ok {
		t.Fatalf("missing ModuleA_Widget in merged schemas: %v", keysOfAny(schemas))
	}

	bWidget, ok := schemas["ModuleB_Widget"]
	if !ok {
		t.Fatalf("missing ModuleB_Widget in merged schemas: %v", keysOfAny(schemas))
	}

	if _, ok := asMap(t, asMap(t, aWidget)["properties"])["a_field"]; !ok {
		t.Fatalf("ModuleA_Widget missing a_field: %v", aWidget)
	}

	if _, ok := asMap(t, asMap(t, bWidget)["properties"])["b_field"]; !ok {
		t.Fatalf("ModuleB_Widget missing b_field: %v", bWidget)
	}
}

// TestMergedSpecConflictingPathsError proves two modules declaring the
// identical HTTP method+path produce a non-nil error naming both
// conflicting RPCs, rather than one silently overwriting the other.
func TestMergedSpecConflictingPathsError(t *testing.T) {
	schema := compileMultiModule(t, map[string]string{
		"schema/ModuleA/widget.zen": `entity Widget {
			id: uuid @primary
		}

		service ModuleAService {
			rpc GetWidget(id: uuid) -> Widget {
				http: GET "/widgets/{id}"
				auth: required
			}
		}`,
		"schema/ModuleB/gadget.zen": `entity Gadget {
			id: uuid @primary
		}

		service ModuleBService {
			rpc GetGadget(id: uuid) -> Gadget {
				http: GET "/widgets/{id}"
				auth: required
			}
		}`,
	})

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("expected an error for conflicting method+path across modules")
	}

	msg := err.Error()
	if !strings.Contains(msg, "ModuleAService.GetWidget") || !strings.Contains(msg, "ModuleBService.GetGadget") {
		t.Fatalf("error %q should name both conflicting RPCs", msg)
	}
}

// TestScalarMappingIsExhaustive iterates every declared ir.ScalarType value
// and asserts scalarOpenAPI returns a non-empty type for each — a new
// scalar type added later must fail this test loudly rather than silently
// falling through to a wrong default.
func TestScalarMappingIsExhaustive(t *testing.T) {
	all := []ir.ScalarType{
		ir.TUUID, ir.TString, ir.TInt32, ir.TInt64, ir.TFloat32, ir.TFloat64,
		ir.TBool, ir.TTimestamp, ir.TDate, ir.TBytes, ir.TJSON, ir.TEnum,
	}

	if int(ir.TEnum)+1 != len(all) {
		t.Fatalf("this test's scalar list (%d entries) is out of sync with ir.ScalarType's enum (last value %d) — update `all`", len(all), ir.TEnum)
	}

	for _, s := range all {
		typ, _ := scalarOpenAPI(s)
		if typ == "" {
			t.Fatalf("scalarOpenAPI(%v) returned an empty type — every ir.ScalarType value must map to a defined {type, format} pair", s)
		}
	}
}

// TestMessageFieldRefRendersAsComponentRef covers Gap B's backend side: a
// message field that references another message (ir.Field.Ref, resolved by
// resolver_message.go) renders as a $ref to that other message's own
// component schema (docBuilder.renderMessageSchema), not as an inline/empty
// scalar schema -- proven both for a message used only in Returns position
// (TokenPairResponse.access -> TokenResponse) and for a message used as a
// whole RPC param, i.e. request position (LoginRequest.token ->
// TokenResponse), which additionally exercises validateFieldsIn recursing
// into the referenced TokenResponse's own field to require @validate there.
func TestMessageFieldRefRendersAsComponentRef(t *testing.T) {
	schema := compileSchema(t, `entity User {
		id: uuid @primary
	}

	message TokenResponse {
		value: string @validate(min_len: 1)
	}

	message TokenPairResponse {
		access: TokenResponse
		refresh: TokenResponse
	}

	message LoginRequest {
		token: TokenResponse
	}

	service AuthService {
		rpc Login(req: LoginRequest) -> TokenPairResponse {
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

	pair := asMap(t, schemas["TokenPairResponse"])
	pairProps := asMap(t, pair["properties"])

	access := asMap(t, pairProps["access"])
	if access["$ref"] != "#/components/schemas/TokenResponse" {
		t.Fatalf("TokenPairResponse.access schema = %v, want $ref #/components/schemas/TokenResponse", access)
	}

	refresh := asMap(t, pairProps["refresh"])
	if refresh["$ref"] != "#/components/schemas/TokenResponse" {
		t.Fatalf("TokenPairResponse.refresh schema = %v, want $ref #/components/schemas/TokenResponse", refresh)
	}

	loginReq := asMap(t, schemas["LoginRequest"])
	loginProps := asMap(t, loginReq["properties"])

	token := asMap(t, loginProps["token"])
	if token["$ref"] != "#/components/schemas/TokenResponse" {
		t.Fatalf("LoginRequest.token schema = %v, want $ref #/components/schemas/TokenResponse", token)
	}

	// TokenResponse itself must still be a plain object schema (not a $ref),
	// with its own "value" field rendered as a normal string schema.
	tokenResp := asMap(t, schemas["TokenResponse"])
	if tokenResp["type"] != "object" {
		t.Fatalf("TokenResponse schema = %v, want type=object", tokenResp)
	}

	valueSchema := asMap(t, asMap(t, tokenResp["properties"])["value"])
	if valueSchema["type"] != "string" || valueSchema["minLength"] != float64(1) {
		t.Fatalf("TokenResponse.value schema = %v, want type=string minLength=1", valueSchema)
	}
}

// TestHTTPPathParams covers the local placeholder scanner directly.
func TestHTTPPathParams(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/v1/orders/{id}", []string{"id"}},
		{"/v1/orders/{order_id}/items/{item_id}", []string{"order_id", "item_id"}},
		{"/v1/orders", nil},
	}

	for _, tt := range tests {
		got := httpPathParams(tt.path)
		if len(got) != len(tt.want) {
			t.Fatalf("httpPathParams(%q) = %v, want %v", tt.path, got, tt.want)
		}

		for _, w := range tt.want {
			if !got[w] {
				t.Fatalf("httpPathParams(%q) missing %q", tt.path, w)
			}
		}
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

func keysOfAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
