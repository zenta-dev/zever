// Coverage tests for the proto backend's enum, request-shape, and option
// branches that the main golden tests do not exercise.
//
// Reachability notes (all branches below are covered; none are left silent):
//   - renderHTTPOption's default branch is defensive: resolveHTTP rejects any
//     method outside GET/DELETE/POST/PUT/PATCH before a schema reaches this
//     backend, so it is covered by a direct unit call, not via Generate.
//   - renderResponseMessage's nil-Returns branch is defensive: the parser
//     requires every rpc to declare a return type, so it is covered by a
//     direct unit call with a hand-built Operation.
//   - protoScalar's TEnum case and trailing return are exhaustiveness guards:
//     renderField never routes enums through protoScalar, and every
//     ir.ScalarType value has an explicit case. Both are covered by a direct
//     table including an out-of-range ScalarType value.
package proto

import (
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// TestName_returnsProto covers Backend.Name, which no golden test calls.
func TestName_returnsProto(t *testing.T) {
	t.Parallel()

	if got := New().Name(); got != "proto" {
		t.Fatalf("Name() = %q, want %q", got, "proto")
	}
}

// TestGenerateNamedEnum proves top-level `enum` declarations render once as
// file-scoped enums (not per-message nested enums) and that fields
// referencing them by name use the shared type. Two enums hit the
// multi-enum separator branch in renderModuleFile.
func TestGenerateNamedEnum(t *testing.T) {
	t.Parallel()

	src := `enum Status { active, inactive }

	enum Priority { low, high }

	entity Task {
		id: uuid @primary
		status: Status
		priority: Priority
		inline: enum(x, y)
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "named_enum", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)
	for _, want := range []string{
		"enum Status {",
		"enum Priority {",
		"Status status = 2;",
		"Priority priority = 3;",
		"enum InlineEnum", // inline enums keep the nested-enum shape.
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered schema.proto missing %q:\n%s", want, rendered)
		}
	}
}

// TestGenerateRequestShapes covers the request-message synthesis branches:
// a single-ref param RPC reuses the referenced message directly, PUT/PATCH
// render body:"*", an enum param gets a nested enum, and a multi-param RPC
// with one ref param embeds the referenced type.
func TestGenerateRequestShapes(t *testing.T) {
	t.Parallel()

	src := `entity Order {
		id: uuid @primary
	}

	message LoginRequest {
		email: string @validate(format: "email")
	}

	message TokenResponse {
		value: string @validate(min_len: 1)
	}

	service Svc {
		rpc Login(req: LoginRequest) -> TokenResponse {
			http: POST "/v1/login"
			auth: required
		}

		rpc PutOrder(id: string @validate(min_len: 1)) -> Order {
			http: PUT "/v1/orders/{id}"
			auth: required
		}

		rpc PatchOrder(id: string @validate(min_len: 1)) -> Order {
			http: PATCH "/v1/orders/{id}"
			auth: required
		}

		rpc SetStatus(id: string @validate(min_len: 1), status: enum(active, archived)) -> Order {
			http: POST "/v1/orders/{id}/status"
			auth: required
		}

		rpc AttachTag(order: Order, tag_id: string @validate(min_len: 1)) -> Order {
			http: POST "/v1/orders/attach"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "request_shapes", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)
	for _, want := range []string{
		"rpc Login(LoginRequest) returns (TokenResponse)",
		`option (google.api.http) = { put: "/v1/orders/{id}" body: "*" };`,
		`option (google.api.http) = { patch: "/v1/orders/{id}" body: "*" };`,
		"enum StatusEnum {",
		"Order order = 1;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered schema.proto missing %q:\n%s", want, rendered)
		}
	}

	if strings.Contains(rendered, "message LoginRequestRequest") {
		t.Fatalf("single-ref RPC must not synthesize a wrapper message:\n%s", rendered)
	}
}

// TestGenerateEdgeOptions covers the option-rendering edges: an `auth: none`
// RPC renders as a bodyless declaration, and a permission-only RPC omits the
// auth option (the nil-auth branch) while still importing annotations.
func TestGenerateEdgeOptions(t *testing.T) {
	t.Parallel()

	src := `entity Order {
		id: uuid @primary
		user_id: uuid
	}

	service Svc {
		rpc GetOrder(id: uuid) -> Order {
			auth: none
		}

		rpc CheckOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			permission: check("owns_order", resource: Order, owner_field: user_id)
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "edge_options", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)
	if !strings.Contains(rendered, "rpc GetOrder(GetOrderRequest) returns (Order);") {
		t.Fatalf("auth:none RPC should render as a bodyless declaration:\n%s", rendered)
	}

	if strings.Contains(rendered, "zengo.annotations.v1.auth") {
		t.Fatalf("no RPC here declares auth, yet an auth option was rendered:\n%s", rendered)
	}

	if !strings.Contains(rendered, `option (zengo.annotations.v1.permission) = { check: "owns_order", resource: "Order", owner_field: "user_id" };`) {
		t.Fatalf("permission-only RPC missing its permission option:\n%s", rendered)
	}
}

// TestGeneratePaginatedResponseNameCollision proves a paginated RPC whose
// synthesized "<Name>Response" message collides with an existing entity is
// reported as a Generate error instead of emitting a duplicate message.
func TestGeneratePaginatedResponseNameCollision(t *testing.T) {
	t.Parallel()

	src := `entity Task {
		id: uuid @primary
	}

	entity ListTasksResponse {
		id: uuid @primary
	}

	service TaskService {
		rpc ListTasks(user_id: uuid) -> Task {
			http: GET "/v1/tasks"
			paginated: true
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err == nil {
		t.Fatalf("Generate: expected a response-name collision error, got nil (output keys %v)", keys(out))
	}

	msg := err.Error()
	if !strings.Contains(msg, "ListTasksResponse") {
		t.Fatalf("Generate error %q does not name the colliding response message", msg)
	}
}

// TestGenerateRequestEnumCollision proves colliding enum values on an RPC
// param are reported as a Generate error naming the RPC.
func TestGenerateRequestEnumCollision(t *testing.T) {
	t.Parallel()

	src := `entity Task {
		id: uuid @primary
	}

	service TaskService {
		rpc SetStatus(id: string @validate(min_len: 1), status: enum(inProgress, in_progress)) -> Task {
			http: POST "/v1/tasks/status"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err == nil {
		t.Fatalf("Generate: expected an enum-value collision error, got nil (output keys %v)", keys(out))
	}

	msg := err.Error()
	if !strings.Contains(msg, "IN_PROGRESS") || !strings.Contains(msg, "TaskService.SetStatus") {
		t.Fatalf("Generate error %q does not name the colliding value and RPC", msg)
	}
}

// TestGenerateMessageEnumCollision proves colliding enum values on a message
// field are reported as a Generate error through the renderMessageForMessage
// path (distinct from the entity-field path TestGenerateEnumValueCollision
// covers).
func TestGenerateMessageEnumCollision(t *testing.T) {
	t.Parallel()

	src := `message Filter {
		status: enum(inProgress, in_progress)
	}

	entity Task {
		id: uuid @primary
	}

	service TaskService {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/v1/tasks/{id}"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err == nil {
		t.Fatalf("Generate: expected an enum-value collision error, got nil (output keys %v)", keys(out))
	}

	if !strings.Contains(err.Error(), "IN_PROGRESS") {
		t.Fatalf("Generate error %q does not name the colliding enum value", err.Error())
	}
}

// TestGenerateEnumEntityNameCollision proves an enum and an entity mapping
// to the same proto message name are reported as a Generate error.
func TestGenerateEnumEntityNameCollision(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	enum := &ir.Enum{Name: "User", Values: []string{"active"}, Module: mod}
	entity := &ir.Entity{
		Name:   "User",
		Module: mod,
		Fields: []*ir.Field{{Name: "id", Primary: true, Type: ir.FieldType{Scalar: ir.TUUID}}},
	}
	mod.Enums = []*ir.Enum{enum}
	mod.Entities = []*ir.Entity{entity}

	if _, err := New().Generate(&ir.Schema{Modules: []*ir.Module{mod}}); err == nil {
		t.Fatal("Generate: expected a message-name collision error, got nil")
	} else if !strings.Contains(err.Error(), "collides") {
		t.Fatalf("Generate error %q does not report a collision", err.Error())
	}
}

// TestGenerateDuplicateTopLevelEnum proves two enums claiming the same proto
// name are reported as a Generate error.
func TestGenerateDuplicateTopLevelEnum(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	mod.Enums = []*ir.Enum{
		{Name: "Status", Values: []string{"active"}, Module: mod},
		{Name: "Status", Values: []string{"inactive"}, Module: mod},
	}

	if _, err := New().Generate(&ir.Schema{Modules: []*ir.Module{mod}}); err == nil {
		t.Fatal("Generate: expected an enum-name collision error, got nil")
	} else if !strings.Contains(err.Error(), "collides") {
		t.Fatalf("Generate error %q does not report a collision", err.Error())
	}
}

// TestGenerateTopLevelEnumValueCollision proves a top-level enum whose
// values collide after normalization is reported as a Generate error. The
// resolver rejects such enums before Generate runs, so this uses hand-built
// IR to reach renderModuleFile's renderTopLevelEnum error branch.
func TestGenerateTopLevelEnumValueCollision(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	mod.Enums = []*ir.Enum{{Name: "Status", Values: []string{"inProgress", "in_progress"}, Module: mod}}

	if _, err := New().Generate(&ir.Schema{Modules: []*ir.Module{mod}}); err == nil {
		t.Fatal("Generate: expected an enum-value collision error, got nil")
	} else if !strings.Contains(err.Error(), "IN_PROGRESS") {
		t.Fatalf("Generate error %q does not name the colliding value", err.Error())
	}
}

// TestRenderTopLevelEnum_duplicateValue proves the dedup guard fires for a
// hand-built enum, covering the error branch Generate cannot reach through
// the resolver (which rejects duplicate values first).
func TestRenderTopLevelEnum_duplicateValue(t *testing.T) {
	t.Parallel()

	_, err := renderTopLevelEnum(&ir.Enum{Name: "Status", Values: []string{"inProgress", "in_progress"}})
	if err == nil {
		t.Fatal("renderTopLevelEnum: expected a collision error, got nil")
	}

	if !strings.Contains(err.Error(), "IN_PROGRESS") {
		t.Fatalf("renderTopLevelEnum error %q does not name the colliding value", err.Error())
	}
}

// TestProtoScalar_exhaustive maps every scalar plus an out-of-range value,
// proving the TEnum guard and the trailing defensive return.
func TestProtoScalar_exhaustive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scalar     ir.ScalarType
		wantType   string
		wantImport string
	}{
		{name: "uuid", scalar: ir.TUUID, wantType: "string"},
		{name: "string", scalar: ir.TString, wantType: "string"},
		{name: "int32", scalar: ir.TInt32, wantType: "int32"},
		{name: "int64", scalar: ir.TInt64, wantType: "int64"},
		{name: "float32", scalar: ir.TFloat32, wantType: "float"},
		{name: "float64", scalar: ir.TFloat64, wantType: "double"},
		{name: "bool", scalar: ir.TBool, wantType: "bool"},
		{name: "timestamp", scalar: ir.TTimestamp, wantType: "google.protobuf.Timestamp", wantImport: importTimestamp},
		{name: "date", scalar: ir.TDate, wantType: "string"},
		{name: "bytes", scalar: ir.TBytes, wantType: "bytes"},
		{name: "json", scalar: ir.TJSON, wantType: "google.protobuf.Struct", wantImport: importStruct},
		{name: "enum", scalar: ir.TEnum, wantType: "", wantImport: ""},
		{name: "invalid", scalar: ir.ScalarType(99), wantType: "", wantImport: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotImport := protoScalar(tt.scalar)
			if gotType != tt.wantType || gotImport != tt.wantImport {
				t.Fatalf("protoScalar(%v) = (%q, %q), want (%q, %q)", tt.scalar, gotType, gotImport, tt.wantType, tt.wantImport)
			}
		})
	}
}

// TestRenderHTTPOption_branches covers every verb branch plus nil and the
// defensive default for methods the resolver never forwards.
func TestRenderHTTPOption_branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		transport *ir.HTTPTransport
		want      string
	}{
		{name: "nil", transport: nil, want: ""},
		{
			name:      "get",
			transport: &ir.HTTPTransport{Method: nethttp.MethodGet, Path: "/v1/orders/{id}"},
			want:      `option (google.api.http) = { get: "/v1/orders/{id}" };`,
		},
		{
			name:      "delete",
			transport: &ir.HTTPTransport{Method: nethttp.MethodDelete, Path: "/v1/orders/{id}"},
			want:      `option (google.api.http) = { delete: "/v1/orders/{id}" };`,
		},
		{
			name:      "post",
			transport: &ir.HTTPTransport{Method: nethttp.MethodPost, Path: "/v1/orders"},
			want:      `option (google.api.http) = { post: "/v1/orders" body: "*" };`,
		},
		{
			name:      "put",
			transport: &ir.HTTPTransport{Method: nethttp.MethodPut, Path: "/v1/orders/{id}"},
			want:      `option (google.api.http) = { put: "/v1/orders/{id}" body: "*" };`,
		},
		{
			name:      "patch",
			transport: &ir.HTTPTransport{Method: nethttp.MethodPatch, Path: "/v1/orders/{id}"},
			want:      `option (google.api.http) = { patch: "/v1/orders/{id}" body: "*" };`,
		},
		{
			name:      "unknown",
			transport: &ir.HTTPTransport{Method: "OPTIONS", Path: "/v1/orders"},
			want:      `option (google.api.http) = { options: "/v1/orders" };`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderHTTPOption(tt.transport); got != tt.want {
				t.Fatalf("renderHTTPOption(%v) = %q, want %q", tt.transport, got, tt.want)
			}
		})
	}
}

// TestRenderAuthOption_branches covers nil (auth:none), required without
// roles, and required with roles, both polarities of the flag.
func TestRenderAuthOption_branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		auth *ir.AuthPolicy
		want string
	}{
		{name: "nil", auth: nil, want: ""},
		{
			name: "required",
			auth: &ir.AuthPolicy{Required: true},
			want: `option (zengo.annotations.v1.auth) = { required: true };`,
		},
		{
			name: "not required",
			auth: &ir.AuthPolicy{Required: false},
			want: `option (zengo.annotations.v1.auth) = { required: false };`,
		},
		{
			name: "roles",
			auth: &ir.AuthPolicy{Required: true, Roles: []string{"owner", "admin"}},
			want: `option (zengo.annotations.v1.auth) = { required: true, roles: ["owner", "admin"] };`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderAuthOption(tt.auth); got != tt.want {
				t.Fatalf("renderAuthOption(%v) = %q, want %q", tt.auth, got, tt.want)
			}
		})
	}
}

// TestRenderResponseMessage_nilReturns proves a paginated operation without
// a declared return type responds with google.protobuf.Empty items.
func TestRenderResponseMessage_nilReturns(t *testing.T) {
	t.Parallel()

	var w strings.Builder

	svc := &ir.Service{Name: "Svc"}
	rpc := &ir.Operation{Name: "List", Paginated: true}

	if err := renderResponseMessage(&w, svc, rpc, newFileCtx()); err != nil {
		t.Fatalf("renderResponseMessage: %v", err)
	}

	got := w.String()
	if !strings.Contains(got, "message ListResponse {") {
		t.Fatalf("response message has the wrong name:\n%s", got)
	}

	if !strings.Contains(got, "repeated google.protobuf.Empty items = 1;") {
		t.Fatalf("nil-Returns response should page over Empty:\n%s", got)
	}
}
