package gogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// TestGeneratedRouterUsesZeverMiddleware is the retarget proof for the HTTP
// transport: generated routes must wrap guarded handlers in zever's
// authz.Middleware (dirty zen-go named it authz.AuthzMiddleware, which does
// not exist in zever) and reference only zever import paths.
func TestGeneratedRouterUsesZeverMiddleware(t *testing.T) {
	out := generatePolicyFixture(t)
	router := string(out["app/router.go"])

	if strings.Contains(router, "AuthzMiddleware") {
		t.Fatalf("router.go references dirty authz.AuthzMiddleware, want zever authz.Middleware:\n%s", router)
	}

	if !strings.Contains(router, "authz.Middleware(a, p, OrderServiceCreateOrderPolicy,") {
		t.Fatalf("router.go missing authz.Middleware wrap for guarded op:\n%s", router)
	}

	for _, want := range []string{
		`"github.com/zenta-dev/zever/core/router"`,
		`"github.com/zenta-dev/zever/shared/apperror"`,
		`"github.com/zenta-dev/zever/core/authz"`,
		`"github.com/zenta-dev/zever/core/auth"`,
		`"github.com/zenta-dev/zever/core/permission"`,
	} {
		if !strings.Contains(router, want) {
			t.Errorf("router.go missing zever import %s", want)
		}
	}

	if strings.Contains(router, "zenta-dev/zen-go") {
		t.Fatalf("router.go still imports dirty zen-go paths")
	}
}

// TestGeneratedGRPCUsesZeverPaths proves grpc.go and register.go reference
// only zever runtime paths plus the standard grpc/protobuf modules.
func TestGeneratedGRPCUsesZeverPaths(t *testing.T) {
	out := generatePolicyFixture(t)

	for _, path := range []string{"app/grpc.go"} {
		got := string(out[path])

		if strings.Contains(got, "zenta-dev/zen-go") {
			t.Errorf("%s still imports dirty zen-go paths", path)
		}

		if !strings.Contains(got, `"github.com/zenta-dev/zever/shared/apperror"`) {
			t.Errorf("%s missing zever apperror import", path)
		}
	}

	register := string(out["app/register.go"])
	if strings.Contains(register, "zenta-dev/zen-go") {
		t.Fatalf("register.go still imports dirty zen-go paths")
	}

	for _, want := range []string{
		`"github.com/zenta-dev/zever/core/router"`,
		`"github.com/zenta-dev/zever/core/auth"`,
		`"github.com/zenta-dev/zever/core/permission"`,
	} {
		if !strings.Contains(register, want) {
			t.Errorf("register.go missing zever import %s", want)
		}
	}

	if got := string(out["app/register.go"]); !strings.Contains(got, `"google.golang.org/grpc"`) {
		t.Fatalf("register.go missing grpc import:\n%s", got)
	}
}

// TestGeneratedGRPCNeverLeaksInternalErrors is the fail-closed proof for the
// gRPC transport: a non-apperror failure (service bug, panic value) must map
// to a fixed "internal error" message, never err.Error(), which could leak
// internals to the client. This is a deliberate delta from dirty zen-go,
// which rendered err.Error().
func TestGeneratedGRPCNeverLeaksInternalErrors(t *testing.T) {
	out := generatePolicyFixture(t)
	grpc := string(out["app/grpc.go"])

	if !strings.Contains(grpc, `status.Error(codes.Internal, "internal error")`) {
		t.Fatalf("grpc.go missing fixed internal-error mapping:\n%s", grpc)
	}

	if strings.Contains(grpc, "status.Error(codes.Internal, err.Error())") {
		t.Fatalf("grpc.go leaks err.Error() to gRPC clients:\n%s", grpc)
	}
}

// TestGeneratedRouterNeverLeaksInternalErrors is the HTTP counterpart:
// unknown errors render a fixed "internal error" body via writeGogenError
// and proto-marshal failures do the same via writeGogenProtoJSON.
func TestGeneratedRouterNeverLeaksInternalErrors(t *testing.T) {
	out := generatePolicyFixture(t)
	router := string(out["app/router.go"])

	if count := strings.Count(router, `"internal error"`); count < 2 {
		t.Fatalf("router.go must render fixed internal-error bodies (>=2), got %d:\n%s", count, router)
	}
}

// TestValidationRunsBeforeServiceCall proves fail-closed request validation
// in both generated transports: validate<Op>Request is invoked after decode
// and before the Service method on every validated operation.
func TestValidationRunsBeforeServiceCall(t *testing.T) {
	out := generateGolden(t)

	router := string(out["app/router.go"])
	validateIdx := strings.Index(router, "validateCreateTaskRequest(req)")
	svcIdx := strings.Index(router, "svc.CreateTask(ctx, req)")

	if validateIdx == -1 {
		t.Fatalf("router.go missing validateCreateTaskRequest call:\n%s", router)
	}

	if svcIdx == -1 {
		t.Fatalf("router.go missing svc.CreateTask call:\n%s", router)
	}

	if validateIdx > svcIdx {
		t.Fatalf("router.go validates after calling the service (must be before)")
	}

	grpc := string(out["app/grpc.go"])
	gValidateIdx := strings.Index(grpc, "validateCreateTaskRequest(req)")
	gSvcIdx := strings.Index(grpc, "g.svc.CreateTask(ctx, req)")

	if gValidateIdx == -1 || gSvcIdx == -1 {
		t.Fatalf("grpc.go missing validate/service calls for CreateTask:\n%s", grpc)
	}

	if gValidateIdx > gSvcIdx {
		t.Fatalf("grpc.go validates after calling the service (must be before)")
	}
}

// TestPaginatedServiceTakesCursorLimit locks the paginated method shape the
// stub must implement: cursor/limit travel outside the protobuf request.
func TestPaginatedServiceTakesCursorLimit(t *testing.T) {
	out := generateGolden(t)

	if !strings.Contains(string(out["app/service.go"]),
		"ListTasks(ctx context.Context, req *ListTasksRequest, cursor string, limit int32) (*ListTasksResponse, error)") {
		t.Fatalf("service.go missing paginated ListTasks shape:\n%s", out["app/service.go"])
	}
}

// TestModuleNamingCoversBothModuleShapes covers moduleNaming/ModuleNaming
// for the implicit unnamed module (app) and a named module.
func TestModuleNamingCoversBothModuleShapes(t *testing.T) {
	pkg, dir := ModuleNaming(nil)
	if pkg != "app" || dir != "app" {
		t.Fatalf("implicit module naming = (%q, %q), want (app, app)", pkg, dir)
	}

	pkg, dir = ModuleNaming(&ir.Module{Name: "billing"})
	if pkg != "billing" || dir != "billing" {
		t.Fatalf("named module naming = (%q, %q), want (billing, billing)", pkg, dir)
	}
}

// TestModuleLabelCoversNilAndNamed covers moduleLabel's readable fallback
// for the implicit module and passthrough for a named one.
func TestModuleLabelCoversNilAndNamed(t *testing.T) {
	if got := moduleLabel(nil); got != "application" {
		t.Fatalf("moduleLabel(nil) = %q, want application", got)
	}

	if got := moduleLabel(&ir.Module{}); got != "application" {
		t.Fatalf("moduleLabel(unnamed) = %q, want application", got)
	}

	if got := moduleLabel(&ir.Module{Name: "billing"}); got != "billing" {
		t.Fatalf("moduleLabel(named) = %q, want billing", got)
	}
}

// TestModuleNeedsAuthzCoversBothBranches covers ModuleNeedsAuthz with and
// without a guarded operation.
func TestModuleNeedsAuthzCoversBothBranches(t *testing.T) {
	if ModuleNeedsAuthz(&ir.Module{Services: []*ir.Service{{Name: "S"}}}) {
		t.Fatalf("ModuleNeedsAuthz must be false with no guarded operations")
	}

	guarded := &ir.Module{Services: []*ir.Service{{Name: "S", Operations: []*ir.Operation{
		{Name: "Op", Auth: &ir.AuthPolicy{Required: true}},
	}}}}

	if !ModuleNeedsAuthz(guarded) {
		t.Fatalf("ModuleNeedsAuthz must be true with a guarded operation")
	}

	permGuarded := &ir.Module{Services: []*ir.Service{{Name: "S", Operations: []*ir.Operation{
		{Name: "Op", Permission: &ir.PermissionCheck{Check: "x.read"}},
	}}}}

	if !ModuleNeedsAuthz(permGuarded) {
		t.Fatalf("ModuleNeedsAuthz must be true with a permission-guarded operation")
	}
}

// TestNewWithPBImportRootEmptyFallsBackToDefault covers the empty-override
// branch: "" reuses the default root (with the flat formula
// NewWithPBImportRoot always applies), instead of producing an empty import.
func TestNewWithPBImportRootEmptyFallsBackToDefault(t *testing.T) {
	file := compileSchema(t, taskFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := NewWithPBImportRoot("").Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	types := string(out["app/types.go"])
	if !strings.Contains(types, `pb "github.com/zenta-dev/zever/gen"`) {
		t.Fatalf("empty override must fall back to the default root, got:\n%s", types)
	}
}
