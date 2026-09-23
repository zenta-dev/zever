package gogen

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	dslparser "github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// compileSchema mirrors zenorm's own test helper: run src through the
// parser and resolver, failing on any diagnostic.
func compileSchema(t *testing.T, src string) *ast.File {
	t.Helper()

	p := dslparser.New("test.zen", []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	return file
}

const taskFixture = `entity Task {
	id: uuid @primary
	title: string
	done: bool
	due_at: timestamp
}

service TaskService {
	rpc ListTasks(user_id: uuid) -> Task {
		http: GET "/tasks"
		auth: required
	}

	rpc CreateTask(title: string, due_at: timestamp) -> Task {
		http: POST "/tasks"
		auth: required
	}

	rpc CompleteTask(id: uuid) -> Task {
		http: PATCH "/tasks/{id}"
		auth: required
		errors: { not_found }
	}

	rpc DeleteTask(id: uuid) -> Task {
		http: DELETE "/tasks/{id}"
		auth: required
	}
}`

func TestGenerateSkipsModuleWithNoServices(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
		name: string
	}`

	file := compileSchema(t, src)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(out) != 0 {
		t.Fatalf("Generate produced output for a module with no services: %v", out)
	}
}

func TestGenerateProducesFiveFilesPerModule(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	want := []string{"app/types.go", "app/service.go", "app/router.go", "app/grpc.go", "app/register.go"}
	for _, path := range want {
		if _, ok := out[path]; !ok {
			t.Errorf("Generate output missing %q", path)
		}
	}

	if len(out) != len(want) {
		t.Fatalf("Generate produced %d files, want %d: %v", len(out), len(want), keysOf(out))
	}
}

// TestRegenerationDoesNotTouchUserImplementationFile confirms the always-
// regenerated gogen output paths never collide with a plausible user
// implementation file path. This is the actual Phase 8 safety property:
// separate files/packages, not in-file markers -- gogen writes only inside
// its own "<module>/{types,service,router,grpc}.go" files, and a user's
// implementation (e.g. "internal/app/task_service.go", or any path outside
// that fixed four-name set) is never among them.
func TestRegenerationDoesNotTouchUserImplementationFile(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	plausibleUserPaths := []string{
		"app/task_service.go",
		"internal/app/task_service_impl.go",
		"app/impl.go",
	}

	for _, p := range plausibleUserPaths {
		if _, ok := out[p]; ok {
			t.Fatalf("Generate unexpectedly wrote a plausible user-implementation path %q", p)
		}
	}

	for path := range out {
		switch path {
		case "app/types.go", "app/service.go", "app/router.go", "app/grpc.go", "app/register.go":
			// expected
		default:
			t.Fatalf("Generate wrote unexpected path %q outside the fixed always-regenerated set", path)
		}
	}
}

// paginatedFixture declares one paginated operation (ListTasks) alongside a
// plain, non-paginated one (GetTask) in the same service, so
// TestGenerateHandlesPaginatedOperation and
// TestNonPaginatedOperationOutputUnaffectedByPaginatedFeature can both
// exercise the mixed case the plan calls for.
const paginatedFixture = `entity Task {
	id: uuid @primary
	title: string
}

service TaskService {
	rpc ListTasks(user_id: uuid) -> Task {
		http: GET "/tasks"
		paginated: true
		auth: required
	}

	rpc GetTask(id: uuid) -> Task {
		http: GET "/tasks/{id}"
		auth: required
	}
}`

// TestGenerateHandlesPaginatedOperation proves a `paginated: true` operation
// generates: a "ListTasksResponse" type alias onto protogogen's own
// synthesized paginated-response message in types.go, a Service interface
// method taking explicit cursor/limit parameters (the real
// "ListTasksRequest" protobuf message carries no such fields) and returning
// *ListTasksResponse, and a router handler that binds cursor/limit query
// params into locals before calling the service.
func TestGenerateHandlesPaginatedOperation(t *testing.T) {
	file := compileSchema(t, paginatedFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["app/types.go"])

	if !strings.Contains(types, "type ListTasksResponse = pb.ListTasksResponse") {
		t.Fatalf("types.go missing ListTasksResponse alias:\n%s", types)
	}

	if !strings.Contains(types, "type ListTasksRequest = pb.ListTasksRequest") {
		t.Fatalf("types.go missing ListTasksRequest alias:\n%s", types)
	}

	service := string(out["app/service.go"])

	if !strings.Contains(service, "ListTasks(ctx context.Context, req *ListTasksRequest, cursor string, limit int32) (*ListTasksResponse, error)") {
		t.Fatalf("service.go ListTasks signature not using cursor/limit params and *ListTasksResponse:\n%s", service)
	}

	router := string(out["app/router.go"])

	if !strings.Contains(router, `cursor = r.URL.Query().Get("cursor")`) {
		t.Fatalf("router.go missing cursor query-param binding:\n%s", router)
	}

	if !strings.Contains(router, `r.URL.Query().Get("limit")`) {
		t.Fatalf("router.go missing limit query-param binding:\n%s", router)
	}

	if !strings.Contains(router, "svc.ListTasks(ctx, req, cursor, limit)") {
		t.Fatalf("router.go handler must call ListTasks with cursor/limit:\n%s", router)
	}
}

// TestNonPaginatedOperationOutputUnaffectedByPaginatedFeature proves a plain
// (non-paginated) operation's generated request alias, interface signature,
// and router handler are unaffected by a sibling paginated operation in the
// same schema/service.
func TestNonPaginatedOperationOutputUnaffectedByPaginatedFeature(t *testing.T) {
	file := compileSchema(t, paginatedFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["app/types.go"])

	if !strings.Contains(types, "type GetTaskRequest = pb.GetTaskRequest") {
		t.Fatalf("GetTaskRequest alias unexpectedly changed:\n%s", types)
	}

	if strings.Contains(types, "GetTaskResponse") {
		t.Fatalf("a non-paginated operation must not get a response wrapper:\n%s", types)
	}

	service := string(out["app/service.go"])

	if !strings.Contains(service, "GetTask(ctx context.Context, req *GetTaskRequest) (*Task, error)") {
		t.Fatalf("GetTask signature unexpectedly changed:\n%s", service)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

// --- compile-test: the generated Service interface is genuinely implementable ---

// fakeTaskService is a minimal hand-written implementation of the interface
// gogen generates for TaskService, proving that interface is implementable
// by ordinary Go code -- the load-bearing property of Phase 7's
// business-logic extension point.
type fakeTaskService struct {
	err  error
	task *task
}

// task is a tiny stand-in for the real protobuf message type protogogen
// generates for the "Task" entity: this test package cannot import a real
// generated gen/<module> package for an ad hoc fixture module, so it
// defines the minimum shape gogen's own generated code needs from it
// (nothing -- gogen never inspects message fields itself, it only imports
// and returns the pointer type by name). Real generated Service interfaces
// return *pb.<Entity> from the sibling protogogen backend; this stand-in is
// only used to prove implementability of a hand-rolled interface literal
// below, not to invoke generated code directly.
type task struct {
	ID string
}

// taskService is the interface shape gogen generates for the taskFixture
// schema above, reproduced by hand so this test can assign fakeTaskService
// to it without depending on a generated package. The real generated
// interface (the fixture's own app/service.go once rendered) has this exact
// method set: pointer request/response types, matching the real protobuf
// messages protogogen generates for the identical schema.
type taskService interface {
	ListTasks(ctx context.Context, req *ListTasksRequest) (*task, error)
	CreateTask(ctx context.Context, req *CreateTaskRequest) (*task, error)
	CompleteTask(ctx context.Context, req *CompleteTaskRequest) (*task, error)
	DeleteTask(ctx context.Context, req *DeleteTaskRequest) (*task, error)
}

type ListTasksRequest struct{ UserID string }
type CreateTaskRequest struct {
	Title string
	DueAt string
}
type CompleteTaskRequest struct{ ID string }
type DeleteTaskRequest struct{ ID string }

func (f *fakeTaskService) ListTasks(context.Context, *ListTasksRequest) (*task, error) {
	return f.task, f.err
}

func (f *fakeTaskService) CreateTask(context.Context, *CreateTaskRequest) (*task, error) {
	return f.task, f.err
}

func (f *fakeTaskService) CompleteTask(context.Context, *CompleteTaskRequest) (*task, error) {
	return f.task, f.err
}

func (f *fakeTaskService) DeleteTask(context.Context, *DeleteTaskRequest) (*task, error) {
	return f.task, f.err
}

func TestGeneratedServiceInterfaceIsImplementable(_ *testing.T) {
	var _ taskService = (*fakeTaskService)(nil)
}

// A handler-level test of the apperror -> HTTP status mapping
// (writeGogenError) against the real generated code can only live inside a
// generated package itself: writeGogenError is unexported generated code.
// The mapping is covered here via golden files (router.go golden contains
// the fixed "internal error" fallback) plus TestWriteGogenErrorContract
// in golden_test.go asserting the emitted helper text.

// --- Phase 3: authz.Policy literal generation ---

// policyFixture declares one operation of each authz shape: auth: required
// with roles, permission: check(...) with no auth, and a plain operation
// declaring neither -- so a single Generate() run proves all three
// generated-code shapes at once.
const policyFixture = `entity Order {
	id: uuid @primary
	user_id: uuid
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/orders/{id}"
		auth: required
	}

	rpc CreateOrder(user_id: uuid) -> Order {
		http: POST "/orders"
		auth: required(roles: {admin})
	}

	rpc DeleteOrder(id: uuid) -> Order {
		http: DELETE "/orders/{id}"
		permission: check("order.delete", resource: Order, owner_field: user_id)
	}
}`

func generatePolicyFixture(t *testing.T) map[string][]byte {
	t.Helper()

	file := compileSchema(t, policyFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return out
}

// TestAuthRequiredWithRolesCompilesToPolicyLiteral proves an
// `auth: required(roles: {admin})` operation compiles to
// authz.Policy{AuthRequired: true, Roles: []string{"admin"}}.
func TestAuthRequiredWithRolesCompilesToPolicyLiteral(t *testing.T) {
	out := generatePolicyFixture(t)
	grpc := string(out["app/grpc.go"])

	if !strings.Contains(grpc, "var OrderServiceCreateOrderPolicy = authz.Policy{") {
		t.Fatalf("grpc.go missing OrderServiceCreateOrderPolicy var:\n%s", grpc)
	}

	if !strings.Contains(grpc, "AuthRequired: true,") {
		t.Fatalf("grpc.go OrderServiceCreateOrderPolicy missing AuthRequired: true:\n%s", grpc)
	}

	if !strings.Contains(grpc, `[]string{"admin"}`) {
		t.Fatalf("grpc.go OrderServiceCreateOrderPolicy missing Roles: []string{\"admin\"}:\n%s", grpc)
	}
}

// TestPermissionCheckCompilesToPolicyLiteral proves a
// `permission: check("order.delete", resource: order, ...)` operation
// compiles to authz.Policy{PermissionCheck: "order.delete", ResourceType:
// "Order"}, with no AuthRequired field emitted (the zero value, false).
func TestPermissionCheckCompilesToPolicyLiteral(t *testing.T) {
	out := generatePolicyFixture(t)
	grpc := string(out["app/grpc.go"])

	if !strings.Contains(grpc, "var OrderServiceDeleteOrderPolicy = authz.Policy{") {
		t.Fatalf("grpc.go missing OrderServiceDeleteOrderPolicy var:\n%s", grpc)
	}

	if !strings.Contains(grpc, `PermissionCheck: "order.delete",`) {
		t.Fatalf("grpc.go OrderServiceDeleteOrderPolicy missing PermissionCheck:\n%s", grpc)
	}

	if !strings.Contains(grpc, `"Order",`) {
		t.Fatalf("grpc.go OrderServiceDeleteOrderPolicy missing ResourceType:\n%s", grpc)
	}

	if strings.Contains(grpc, "OrderServiceDeleteOrderPolicy = authz.Policy{\n\tAuthRequired") {
		t.Fatalf("grpc.go OrderServiceDeleteOrderPolicy must not declare AuthRequired (not requested):\n%s", grpc)
	}
}

// TestOperationWithNeitherAuthNorPermissionGetsNoPolicy proves an operation
// declaring neither auth: nor permission: gets no <Svc><Op>Policy var, no
// Authorize call wrapping its HTTP handler, and no entry in GRPCPolicies().
// TestOperationWithNeitherAuthNorPermissionGetsNoPolicy builds its schema
// directly against ir types, bypassing resolver.Resolve: the v-next
// secure-everything opinion (internal/dsl/resolver/resolver_opinions.go)
// now hard-rejects any operation declaring neither Auth nor Permission at
// compile time, so a real .zen source can no longer reach this shape.
// gogen's own Generate contract is still "no Auth/Permission on the IR ->
// no policy var, no wrapped route" regardless of how the *ir.Schema was
// built, and that is what this test proves.
func TestOperationWithNeitherAuthNorPermissionGetsNoPolicy(t *testing.T) {
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
		Params:     []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}},
		Transports: []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: "GET", Path: "/orders/{id}"}},
	}
	svc := &ir.Service{Name: "OrderService", Module: module, Operations: []*ir.Operation{op}}
	module.Services = []*ir.Service{svc}

	schema := &ir.Schema{Modules: []*ir.Module{module}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	grpc := string(out["app/grpc.go"])
	router := string(out["app/router.go"])

	if strings.Contains(grpc, "OrderServiceGetOrderPolicy") {
		t.Fatalf("grpc.go must not declare a policy var for an unguarded operation:\n%s", grpc)
	}

	if !strings.Contains(router, `r.Handle("GET", "/orders/{id}", handleOrderServiceGetOrder(svc))`) {
		t.Fatalf("router.go must register the unguarded GetOrder route unwrapped:\n%s", router)
	}
}

// TestGRPCPoliciesMapKeyedByFullMethodName proves GRPCPolicies() keys every
// guarded operation's policy by the real generated pb.<Service>_<Method>_FullMethodName
// constant -- the exact string grpc.UnaryServerInfo.FullMethod carries at
// runtime, per authz.UnaryServerInterceptor's own contract.
func TestGRPCPoliciesMapKeyedByFullMethodName(t *testing.T) {
	out := generatePolicyFixture(t)
	grpc := string(out["app/grpc.go"])

	if !strings.Contains(grpc, "func GRPCPolicies() map[string]authz.Policy {") {
		t.Fatalf("grpc.go missing GRPCPolicies():\n%s", grpc)
	}

	if !strings.Contains(grpc, "pb.OrderService_CreateOrder_FullMethodName: OrderServiceCreateOrderPolicy,") {
		t.Fatalf("GRPCPolicies() missing CreateOrder entry:\n%s", grpc)
	}

	if !strings.Contains(grpc, "pb.OrderService_DeleteOrder_FullMethodName: OrderServiceDeleteOrderPolicy,") {
		t.Fatalf("GRPCPolicies() missing DeleteOrder entry:\n%s", grpc)
	}

	// GetOrder declares auth: required (every operation must, per the
	// v-next secure-everything opinion -- see
	// TestOperationWithNeitherAuthNorPermissionGetsNoPolicy for the
	// "no Auth/Permission on the IR -> no entry" case, built directly
	// against ir types since a real .zen source can no longer produce it),
	// so it gets its own policy entry too.
	if !strings.Contains(grpc, "pb.OrderService_GetOrder_FullMethodName:    OrderServiceGetOrderPolicy,") {
		t.Fatalf("GRPCPolicies() missing GetOrder entry:\n%s", grpc)
	}
}

// TestNewDefaultPBImportPathUnchanged locks in that New()'s zero-config
// output uses the legacy "/zever/"-prefixed formula, not the flat one
// NewWithPBImportRoot uses. No zever gen package exists yet, so this
// default is override-required for real projects (see defaultPBImportRoot).
func TestNewDefaultPBImportPathUnchanged(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["app/types.go"])

	if !strings.Contains(types, `pb "github.com/zenta-dev/zever/gen/zeverv1"`) {
		t.Fatalf("New()'s default import path changed, want the legacy zeverv1 convention:\n%s", types)
	}
}

// TestNewWithPBImportRootUsesFlatFormula is the regression case a real
// external project (schema/v1/iam/*.zen) hit: gogen's generated code must
// import protogogen's actual "paths=source_relative" output layout
// (<root>/<module>), not the "/zever/"-prefixed convention that's specific
// to this repo's own committed examples.
func TestNewWithPBImportRootUsesFlatFormula(t *testing.T) {
	file := compileSchema(t, `entity Task {
		id: uuid @primary
	}

	service TaskService {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/tasks/{id}"
			auth: required
		}
	}`)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := NewWithPBImportRoot("api/generated/protogogen").Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["app/types.go"])

	if strings.Contains(types, "/zever/") || strings.Contains(types, "zeverv1") {
		t.Fatalf("NewWithPBImportRoot must not apply the /zever/-prefixed formula:\n%s", types)
	}

	if !strings.Contains(types, `pb "api/generated/protogogen"`) {
		t.Fatalf("expected the flat root as-is for the implicit module, got:\n%s", types)
	}
}

// TestNewWithPBImportRootNamedModuleIsFlat is the named-module counterpart:
// "<root>/<module>", no "/zever/" segment inserted.
func TestNewWithPBImportRootNamedModuleIsFlat(t *testing.T) {
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

	out, err := NewWithPBImportRoot("api/generated/protogogen").Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["iam/types.go"])
	if types == "" {
		t.Fatalf("no output for module iam, got keys: %v", func() []string {
			keys := make([]string, 0, len(out))
			for k := range out {
				keys = append(keys, k)
			}

			return keys
		}())
	}

	if !strings.Contains(types, `pb "api/generated/protogogen/iam"`) {
		t.Fatalf("expected flat <root>/<module> import, got:\n%s", types)
	}
}

// TestGenerateSingleRefParamValidateAddressesFieldsDirectly is the
// regression case a real external project hit: when an rpc's sole param is
// a message/entity ref (the common message-as-param shape), proto skips
// synthesizing a "<Op>Request" wrapper around it -- the ref'd type IS the
// request type directly (see proto's singleRefParam) -- so the generated
// validator must address that type's fields directly on req ("req.<Field>"),
// never nested under the param's own name ("req.<Param>.<Field>"), which
// doesn't exist on the flat message and fails to compile.
func TestGenerateSingleRefParamValidateAddressesFieldsDirectly(t *testing.T) {
	src := `message SignUpRequest {
		name: string @validate(min_len: 2, max_len: 128)
		image_id: string? @validate(format: "uuid")
	}

	message Empty {}

	service AuthService {
		rpc SignUp(req: SignUpRequest) -> Empty {
			auth: none
		}
	}`

	file := compileSchema(t, src)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/grpc.go"])

	if strings.Contains(got, "req.Req") {
		t.Fatalf("validator still addresses fields through a nonexistent wrapper (req.Req.*):\n%s", got)
	}

	if !strings.Contains(got, "func validateSignUpRequest(req *SignUpRequest) error {") {
		t.Fatalf("expected validateSignUpRequest to take *SignUpRequest directly:\n%s", got)
	}

	if !strings.Contains(got, "if req == nil {") {
		t.Fatalf("expected a direct nil guard on req itself, got:\n%s", got)
	}

	if !strings.Contains(got, "if len(req.Name) < 2 {") {
		t.Fatalf("expected direct field access req.Name, got:\n%s", got)
	}

	// image_id is optional (string?): the real protobuf field is *string,
	// so the validate check must nil-guard and dereference, not use it as
	// a plain string directly.
	if !strings.Contains(got, "if req.ImageId != nil {") {
		t.Fatalf("expected a nil guard for the optional ImageId field, got:\n%s", got)
	}

	if !strings.Contains(got, "uuid.Parse(*req.ImageId)") {
		t.Fatalf("expected a dereferenced check for the optional ImageId field, got:\n%s", got)
	}
}

// TestGenerateMultiParamRefStillUsesNestedAccess covers the case
// SingleRefParam does NOT apply: an operation with more than one param,
// one of which is a ref, still gets the "<Op>Request" wrapper (proto only
// skips synthesis for the single-ref-param shape), so field access stays
// nested under the param's own name.
func TestGenerateMultiParamRefStillUsesNestedAccess(t *testing.T) {
	src := `message Profile {
		bio: string @validate(max_len: 500)
	}

	entity User {
		id: uuid @primary
	}

	service UserService {
		rpc UpdateProfile(id: uuid, profile: Profile) -> User {
			auth: required
		}
	}`

	file := compileSchema(t, src)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/grpc.go"])

	if !strings.Contains(got, "func validateUpdateProfileRequest(req *UpdateProfileRequest) error {") {
		t.Fatalf("expected the synthesized wrapper request type, got:\n%s", got)
	}

	if !strings.Contains(got, "if req.Profile == nil {") {
		t.Fatalf("expected a nested nil guard req.Profile, got:\n%s", got)
	}

	if !strings.Contains(got, "if len(req.Profile.Bio) > 500 {") {
		t.Fatalf("expected nested field access req.Profile.Bio, got:\n%s", got)
	}
}

// --- register.go: ModuleImpls / RegisterModule ---

// TestRegisterModuleSingleServiceWithAuthz proves register.go's ModuleImpls
// carries one field per service and RegisterModule takes auth.Auth/
// permission.Checker (mirroring router.go's own withAuthz gate) when the
// module has at least one guarded operation, wiring both HTTP routes and the
// gRPC adapter for each service.
func TestRegisterModuleSingleServiceWithAuthz(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/register.go"])

	if !strings.Contains(got, "type ModuleImpls struct {\n\tTaskService TaskService\n}") {
		t.Fatalf("expected ModuleImpls with one TaskService field, got:\n%s", got)
	}

	if !strings.Contains(got, "func RegisterModule(r router.Router, grpcServer *grpc.Server, a auth.Auth, p permission.Checker, impls ModuleImpls) {") {
		t.Fatalf("expected withAuthz RegisterModule signature, got:\n%s", got)
	}

	if !strings.Contains(got, "RegisterTaskServiceRoutes(r, impls.TaskService, a, p)") {
		t.Fatalf("expected RegisterTaskServiceRoutes call, got:\n%s", got)
	}

	if !strings.Contains(got, "pb.RegisterTaskServiceServer(grpcServer, NewTaskServiceGRPCServer(impls.TaskService))") {
		t.Fatalf("expected gRPC server wiring, got:\n%s", got)
	}
}

// multiServiceFixture declares two services (TaskService, OrderService) in
// the same module, in that source order, so
// TestRegisterModuleMultiServicePreservesDeclarationOrder can assert
// ModuleImpls/RegisterModule both preserve schema declaration order.
const multiServiceFixture = `entity Task {
	id: uuid @primary
	title: string
}

entity Order {
	id: uuid @primary
	user_id: uuid
}

service TaskService {
	rpc GetTask(id: uuid) -> Task {
		http: GET "/tasks/{id}"
		auth: required
	}
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/orders/{id}"
		auth: required
	}
}`

func TestRegisterModuleMultiServicePreservesDeclarationOrder(t *testing.T) {
	file := compileSchema(t, multiServiceFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/register.go"])

	wantImpls := "type ModuleImpls struct {\n\tTaskService TaskService\n\tOrderService OrderService\n}"
	if !strings.Contains(strings.ReplaceAll(got, " ", ""), strings.ReplaceAll(wantImpls, " ", "")) {
		t.Fatalf("expected ModuleImpls fields in declaration order (TaskService, OrderService), got:\n%s", got)
	}

	taskIdx := strings.Index(got, "RegisterTaskServiceRoutes")
	orderIdx := strings.Index(got, "RegisterOrderServiceRoutes")

	if taskIdx == -1 || orderIdx == -1 || taskIdx > orderIdx {
		t.Fatalf("expected TaskService wiring before OrderService wiring, got:\n%s", got)
	}

	if !strings.Contains(got, "pb.RegisterOrderServiceServer(grpcServer, NewOrderServiceGRPCServer(impls.OrderService))") {
		t.Fatalf("expected OrderService gRPC wiring, got:\n%s", got)
	}
}

// TestRegisterModuleNoAuthzOmitsAuthParams proves RegisterModule drops the
// auth.Auth/permission.Checker params (and their imports) entirely when the
// module has no guarded operation at all -- mirroring
// TestOperationWithNeitherAuthNorPermissionGetsNoPolicy's raw-ir construction
// technique, since the DSL's hard-error secureEverything rule means an
// unguarded operation can only be expressed via direct ir construction, not
// parsed .zen source.
func TestRegisterModuleNoAuthzOmitsAuthParams(t *testing.T) {
	entity := &ir.Entity{
		Name: "Order",
		Fields: []*ir.Field{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true},
		},
	}
	module := &ir.Module{Entities: []*ir.Entity{entity}}
	entity.Module = module

	op := &ir.Operation{
		Name:       "GetOrder",
		Returns:    &ir.TypeRef{Entity: entity},
		Params:     []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}},
		Transports: []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: "GET", Path: "/orders/{id}"}},
	}
	svc := &ir.Service{Name: "OrderService", Module: module, Operations: []*ir.Operation{op}}
	module.Services = []*ir.Service{svc}

	schema := &ir.Schema{Modules: []*ir.Module{module}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["app/register.go"])

	if !strings.Contains(got, "func RegisterModule(r router.Router, grpcServer *grpc.Server, impls ModuleImpls) {") {
		t.Fatalf("expected no-authz RegisterModule signature, got:\n%s", got)
	}

	if strings.Contains(got, "auth.Auth") || strings.Contains(got, "permission.Checker") {
		t.Fatalf("no-authz register.go must not reference auth.Auth/permission.Checker, got:\n%s", got)
	}

	if !strings.Contains(got, "RegisterOrderServiceRoutes(r, impls.OrderService)") {
		t.Fatalf("expected 2-arg RegisterOrderServiceRoutes call, got:\n%s", got)
	}
}

// --- GenerateServiceStub ---

// TestGenerateServiceStubCompiles proves GenerateServiceStub emits a
// syntactically valid Go file (format.Source parses+formats it, the same
// check every other gogen renderer already relies on) implementing the
// interface via a compile-time assertion, with every method returning
// apperror.Unimplemented.
func TestGenerateServiceStubCompiles(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	svc := schema.Modules[0].Services[0]

	got, err := GenerateServiceStub("app", "genapp", "example.com/internal/gen/app", svc)
	if err != nil {
		t.Fatalf("GenerateServiceStub: %v", err)
	}

	s := string(got)

	if !strings.Contains(s, "type TaskServiceImpl struct{}") {
		t.Fatalf("expected TaskServiceImpl struct, got:\n%s", s)
	}

	if !strings.Contains(s, "var _ genapp.TaskService = (*TaskServiceImpl)(nil)") {
		t.Fatalf("expected compile-time interface assertion, got:\n%s", s)
	}

	if !strings.Contains(s, `genapp "example.com/internal/gen/app"`) {
		t.Fatalf("expected aliased import of the generated package, got:\n%s", s)
	}

	for _, op := range []string{"ListTasks", "CreateTask", "CompleteTask", "DeleteTask"} {
		if !strings.Contains(s, "func (s *TaskServiceImpl) "+op+"(") {
			t.Fatalf("expected stub method for %s, got:\n%s", op, s)
		}

		if !strings.Contains(s, `apperror.New(apperror.Unimplemented, "`+op+` not implemented")`) {
			t.Fatalf("expected Unimplemented body for %s, got:\n%s", op, s)
		}
	}

	if !strings.Contains(s, "// Code generated") || strings.Contains(s, "DO NOT EDIT") {
		t.Fatalf("stub header must not say DO NOT EDIT (it's meant to be edited), got:\n%s", s)
	}
}

// TestGenerateServiceStubPaginatedSignature proves a paginated operation's
// stub method takes the explicit cursor/limit params service.go's own
// interface declares, not just req.
func TestGenerateServiceStubPaginatedSignature(t *testing.T) {
	file := compileSchema(t, paginatedFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	svc := schema.Modules[0].Services[0]

	got, err := GenerateServiceStub("app", "genapp", "example.com/internal/gen/app", svc)
	if err != nil {
		t.Fatalf("GenerateServiceStub: %v", err)
	}

	s := string(got)

	if !strings.Contains(s, "func (s *TaskServiceImpl) ListTasks(ctx context.Context, req *genapp.ListTasksRequest, cursor string, limit int32) (*genapp.ListTasksResponse, error) {") {
		t.Fatalf("expected paginated stub signature, got:\n%s", s)
	}
}
